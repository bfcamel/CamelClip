package main

import (
	"context"
	"fmt"
	"strings"
)

func videoCodec(o options) string {
	if o.Codec == "" {
		return "h264"
	}
	return o.Codec
}
func videoContainer(o options) string {
	if o.Container == "" {
		return "mp4"
	}
	return o.Container
}
func codecLabel(codec string) string {
	return map[string]string{"h264": "H.264", "hevc": "H.265 / HEVC", "vp9": "VP9", "av1": "AV1"}[codec]
}
func videoCodecChoices() []choice {
	return []choice{{"H.264", "h264"}, {"H.265 / HEVC", "hevc"}, {"VP9", "vp9"}, {"AV1", "av1"}}
}
func videoContainerChoices(codec string) []choice {
	values := []choice{{"MP4", "mp4"}, {"MKV", "mkv"}}
	if codec == "vp9" || codec == "av1" {
		values = append(values, choice{"WebM", "webm"})
	}
	return values
}
func validateVideoProfile(o options) error {
	codec, container := videoCodec(o), videoContainer(o)
	if !contains([]string{"h264", "hevc", "vp9", "av1"}, codec) {
		return fmt.Errorf(tr("неподдерживаемый видеокодек: %s"), codec)
	}
	if !contains([]string{"mp4", "mkv", "webm"}, container) {
		return fmt.Errorf(tr("неподдерживаемый видеоформат: %s"), container)
	}
	if container == "webm" && codec != "vp9" && codec != "av1" {
		return fmt.Errorf(tr("WebM поддерживает только VP9 или AV1"))
	}
	return nil
}
func conversionArguments(source, target string, info probeResult, o options) ([]string, error) {
	if err := validateVideoProfile(o); err != nil {
		return nil, err
	}
	codec, container := videoCodec(o), videoContainer(o)
	encoder := map[string]string{"h264": "libx264", "hevc": "libx265", "vp9": "libvpx-vp9", "av1": "libsvtav1"}[codec]
	audioCodec, audioEncoder := "aac", "aac"
	if container == "webm" {
		audioCodec, audioEncoder = "opus", "libopus"
	}
	videoCopy, audioCopy := false, false
	// Only the selected first video/audio streams determine conversion requirements.
	seenVideo, seenAudio := false, false
	for _, stream := range info.Streams {
		if stream.Type == "video" && !seenVideo {
			seenVideo = true
			videoCopy = stream.Codec == codec && stream.Pixel == "yuv420p"
		}
		if stream.Type == "audio" && !seenAudio {
			seenAudio = true
			audioCopy = stream.Codec == audioCodec
		}
	}
	if videoCopy {
		encoder = "copy"
	}
	if audioCopy {
		audioEncoder = "copy"
	}
	args := []string{"-v", "error", "-xerror", "-nostdin", "-y", "-progress", "pipe:1", "-nostats", "-i", source, "-map", "0:v:0", "-map", "0:a:0", "-c:v", encoder, "-c:a", audioEncoder}
	if !videoCopy {
		args = append(args, "-pix_fmt", "yuv420p")
		switch codec {
		case "h264":
			args = append(args, "-preset", "fast", "-crf", "20")
		case "hevc":
			args = append(args, "-preset", "fast", "-crf", "24")
		case "vp9":
			args = append(args, "-b:v", "0", "-crf", "30", "-deadline", "good", "-cpu-used", "4")
		case "av1":
			args = append(args, "-preset", "8", "-crf", "30")
		}
	}
	if !audioCopy {
		args = append(args, "-b:a", "192k")
	}
	if container == "mp4" {
		args = append(args, "-movflags", "+faststart")
		if codec == "hevc" {
			args = append(args, "-tag:v", "hvc1")
		}
	}
	return append(args, target), nil
}

func verifyVideoProfile(ctx context.Context, path string, o options) error {
	info, err := probeMedia(ctx, path, true, o.duration)
	if err != nil {
		return err
	}
	codec, container := videoCodec(o), videoContainer(o)
	expectedAudio := "aac"
	if container == "webm" {
		expectedAudio = "opus"
	}
	video, audio := false, false
	for _, stream := range info.Streams {
		if stream.Type == "video" && !video {
			video = true
			if stream.Codec != codec || stream.Pixel != "yuv420p" {
				return fmt.Errorf("output does not match selected video codec %s", codec)
			}
		}
		if stream.Type == "audio" && !audio {
			audio = true
			if stream.Codec != expectedAudio {
				return fmt.Errorf("output does not match expected audio codec %s", expectedAudio)
			}
		}
	}
	if container == "mp4" && !strings.Contains(info.Format.Name, "mp4") {
		return fmt.Errorf("output is not MP4")
	}
	if container != "mp4" && !strings.Contains(info.Format.Name, "matroska") {
		return fmt.Errorf("output is not Matroska/WebM")
	}
	return nil
}
