package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type probeResult struct {
	Streams []struct {
		Type  string `json:"codec_type"`
		Codec string `json:"codec_name"`
		Pixel string `json:"pix_fmt"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
		Name     string `json:"format_name"`
	} `json:"format"`
}

func probeMedia(ctx context.Context, path string, video bool, expected float64) (probeResult, error) {
	var info probeResult
	c := command(ctx, "ffprobe", "-v", "error", "-show_streams", "-show_format", "-of", "json", path)
	data, err := c.Output()
	if err != nil {
		return info, fmt.Errorf("cannot read media: %w", err)
	}
	if err = json.Unmarshal(data, &info); err != nil {
		return info, err
	}
	audio, visual := false, false
	for _, s := range info.Streams {
		audio = audio || s.Type == "audio"
		visual = visual || s.Type == "video"
	}
	duration, _ := strconv.ParseFloat(info.Format.Duration, 64)
	if !audio || (video && !visual) || duration <= 0 {
		return info, fmt.Errorf("media has missing streams or invalid duration")
	}
	if expected > 0 && math.Abs(duration-expected) > math.Max(3, expected*0.05) {
		return info, fmt.Errorf("media duration %.1fs differs from expected %.1fs", duration, expected)
	}
	return info, nil
}
func verifyMedia(ctx context.Context, path string, video bool, expected float64) error {
	return verifyMediaWithObserver(ctx, path, video, expected, nil)
}
func verifyMediaWithObserver(ctx context.Context, path string, video bool, expected float64, observer pipelineObserver) error {
	info, err := probeMedia(ctx, path, video, expected)
	if err != nil {
		return err
	}
	if expected <= 0 {
		expected, _ = strconv.ParseFloat(info.Format.Duration, 64)
	}
	a := []string{"-v", "error", "-xerror", "-nostdin", "-progress", "pipe:1", "-nostats", "-i", path, "-map", "0:a:0"}
	if video {
		a = append(a, "-map", "0:v:0")
	}
	c := command(ctx, "ffmpeg", append(a, "-f", "null", "-")...)
	var tail tailBuffer
	c.Stderr = &tail
	c.Stdout = &ffmpegProgress{duration: expected, stage: "verifying", observer: observer}
	if err := c.Run(); err != nil {
		return fmt.Errorf("media validation failed: %w: %s", err, tail.String())
	}
	return nil
}
func compatibleMP4(ctx context.Context, path, dir string, expected float64) (string, error) {
	o := options{duration: expected}
	return convertVideo(ctx, path, dir, o, nil)
}
func convertVideo(ctx context.Context, path, dir string, o options, observer pipelineObserver) (string, error) {
	info, err := probeMedia(ctx, path, true, o.duration)
	if err != nil {
		return "", err
	}
	args, err := conversionArguments(path, filepath.Join(dir, "converted."+videoContainer(o)), info, o)
	if err != nil {
		return "", err
	}
	if o.duration <= 0 {
		o.duration, _ = strconv.ParseFloat(info.Format.Duration, 64)
	}
	target := args[len(args)-1]
	c := command(ctx, "ffmpeg", args...)
	var tail tailBuffer
	c.Stderr = &tail
	c.Stdout = &ffmpegProgress{duration: o.duration, stage: "processing", observer: observer}
	if err = c.Run(); err != nil {
		return "", fmt.Errorf("video conversion failed: %w: %s", err, tail.String())
	}
	return target, nil
}

// Each request has a private staging area. Only fully validated files enter the library.
// A filesystem lock protects partial files even across independent MacDown processes.
func verifiedDownload(ctx context.Context, executable string, o options, output io.Writer) error {
	return verifiedDownloadWithObserver(ctx, executable, o, output, nil)
}
func verifiedDownloadWithObserver(ctx context.Context, executable string, o options, output io.Writer, observer pipelineObserver) error {
	if err := validateVideoProfile(o); err != nil {
		return err
	}
	notifyStage(observer, output, "downloading")
	absoluteOut, err := filepath.Abs(o.Out)
	if err != nil {
		return err
	}
	o.Out = absoluteOut
	if err := os.MkdirAll(o.Out, 0755); err != nil {
		return err
	}
	key := sha256.Sum256([]byte(o.URL + "\x00" + outputVariant(o)))
	stage := filepath.Join(o.Out, fmt.Sprintf(".macdown-%x", key[:12]))
	if err := os.MkdirAll(stage, 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(stage, "lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = lockDownload(ctx, lock); err != nil {
		return err
	}
	ctx = context.WithValue(ctx, stageLockContextKey{}, lock)
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	// Keep the lock inode and directory: removing them could allow another process to bypass the lock.
	manifest := filepath.Join(stage, "result.jsonl")
	if err = os.WriteFile(manifest, nil, 0600); err != nil {
		return err
	}
	staged := o
	staged.Out = stage
	args := arguments(staged)
	args = append(args[:len(args)-2], "--print-to-file", "after_move:%(filepath)j", manifest, "--", o.URL)
	c := command(ctx, executable, args...)
	var tail tailBuffer
	w := &downloaderOutput{out: io.MultiWriter(output, &tail), observer: observer}
	c.Stdout = w
	c.Stderr = w
	if err = c.Run(); err != nil {
		if ctx.Err() != nil {
			return errCanceled
		}
		return fmt.Errorf("%s", explain(tail.String()))
	}
	data, err := os.ReadFile(manifest)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 || lines[0] == "" {
		return fmt.Errorf("downloader did not report exactly one completed file")
	}
	var source string
	if err = json.Unmarshal([]byte(lines[0]), &source); err != nil {
		return err
	}
	source, err = filepath.Abs(source)
	if err != nil {
		return err
	}
	stageAbs, err := filepath.Abs(stage)
	if err != nil {
		return err
	}
	if filepath.Dir(source) != stageAbs {
		return fmt.Errorf("downloader output is outside staging directory")
	}
	st, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() {
		return fmt.Errorf("downloader output is not a regular file")
	}
	result := source
	if o.Mode == "video" {
		notifyStage(observer, output, "processing")
		result, err = convertVideo(ctx, source, stageAbs, o, observer)
		if err != nil {
			return err
		}
	}
	notifyStage(observer, output, "verifying")
	if err = verifyMediaWithObserver(ctx, result, o.Mode == "video", o.duration, observer); err != nil {
		if ctx.Err() != nil {
			return errCanceled
		}
		return err
	}
	if o.Mode == "video" {
		if err = verifyVideoProfile(ctx, result, o); err != nil {
			return err
		}
	}
	if ctx.Err() != nil {
		return errCanceled
	}
	name := filepath.Base(source)
	if o.Mode == "video" {
		name = strings.TrimSuffix(name, filepath.Ext(name)) + "." + videoContainer(o)
	}
	target := filepath.Join(o.Out, name)
	// link publishes atomically without overwriting a file created by another job/process.
	if err = os.Link(result, target); err != nil {
		if !os.IsExist(err) {
			return err
		}
		if err = verifyMedia(ctx, target, o.Mode == "video", o.duration); err != nil {
			return fmt.Errorf("existing file is invalid; kept unchanged: %s: %w", target, err)
		}
	}
	if o.Mode == "video" {
		if err = verifyVideoProfile(ctx, target, o); err != nil {
			return err
		}
	}
	entries, _ := os.ReadDir(stage)
	for _, entry := range entries {
		if entry.Name() != "lock" {
			_ = os.Remove(filepath.Join(stage, entry.Name()))
		}
	}
	fmt.Fprintln(output, tr("Сохранено:"), target)
	return nil
}

// Keep the staging lock in subprocesses too: after an abrupt parent exit an orphan
// downloader/converter must finish before a restarted instance touches its files.
type stageLockContextKey struct{}

func lockDownload(ctx context.Context, lock *os.File) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			return err
		}
		select {
		case <-ctx.Done():
			return errCanceled
		case <-ticker.C:
		}
	}
}
