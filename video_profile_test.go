package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestVideoProfileSettingsMigrationAndCLI(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "settings.json")
	os.WriteFile(path, []byte(`{"language":"ru","mode":"video","quality":"best","audio_format":"mp3","output_directory":"/tmp/videos"}`), 0600)
	o := defaultOptions(root)
	if err := loadSettings(path, &o); err != nil {
		t.Fatal(err)
	}
	if o.Codec != "h264" || o.Container != "mp4" {
		t.Fatal("old settings lost defaults")
	}
	if _, err := parseCLI([]string{"download", "--video-codec", "vp9", "https://example.com/a", "--video-format", "webm"}, &o); err != nil {
		t.Fatal(err)
	}
	if o.Codec != "vp9" || o.Container != "webm" {
		t.Fatal(o)
	}
	if err := saveSettings(path, o); err != nil {
		t.Fatal(err)
	}
	loaded := defaultOptions(root)
	if err := loadSettings(path, &loaded); err != nil || loaded.Codec != "vp9" || loaded.Container != "webm" {
		t.Fatal(loaded, err)
	}
	loaded.Codec = "h264"
	if validateVideoProfile(loaded) == nil {
		t.Fatal("H264/WebM accepted")
	}
}
func TestAllVideoProfilesProduceSelectedCodecsAndContainers(t *testing.T) {
	ctx := context.Background()
	fixture := makeFixture(t)
	base, err := convertVideo(ctx, fixture, t.TempDir(), options{duration: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	encoders, _ := exec.Command("ffmpeg", "-hide_banner", "-encoders").CombinedOutput()
	for _, codec := range []string{"h264", "hevc", "vp9", "av1"} {
		for _, container := range videoContainerChoices(codec) {
			t.Run(codec+"/"+container.Value, func(t *testing.T) {
				encoder := map[string]string{"h264": "libx264", "hevc": "libx265", "vp9": "libvpx-vp9", "av1": "libsvtav1"}[codec]
				if !strings.Contains(string(encoders), encoder) {
					t.Skip("encoder unavailable: " + encoder)
				}
				o := options{Codec: codec, Container: container.Value, duration: 1}
				result, err := convertVideo(ctx, base, t.TempDir(), o, nil)
				if err != nil {
					t.Fatal(err)
				}
				if err = verifyMedia(ctx, result, true, 1); err != nil {
					t.Fatal(err)
				}
				if err = verifyVideoProfile(ctx, result, o); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
func TestPipelineProgressAndMergerStages(t *testing.T) {
	var events []string
	observer := func(stage, detail string) { events = append(events, stage+":"+detail) }
	p := &ffmpegProgress{duration: 10, stage: "processing", observer: observer}
	p.Write([]byte("out_ti"))
	p.Write([]byte("me_us=5000000\nout_time_us=12000000\n"))
	if !reflect.DeepEqual(events, []string{"processing:50%", "processing:99%"}) {
		t.Fatal(events)
	}
	events = nil
	var output bytes.Buffer
	w := &downloaderOutput{out: &output, observer: observer}
	w.Write([]byte("[Mer"))
	w.Write([]byte("ger] Combining\n[VideoRemuxer] Done\n"))
	if !reflect.DeepEqual(events, []string{"processing:"}) {
		t.Fatal(events)
	}
}
func TestCodecAndContainerUISettingsPersist(t *testing.T) {
	root := t.TempDir()
	o := defaultOptions(root)
	o.Codec = "av1"
	o.Container = "webm"
	u := panelUI{ctx: context.Background(), o: &o, home: root, config: filepath.Join(root, "settings.json"), system: "ru", pool: newJobPool(context.Background(), "")}
	u.showChoices("video-codec", "codec", videoCodecChoices(), "h264")
	u.modalKey("enter")
	if o.Codec != "h264" || o.Container != "mkv" {
		t.Fatal(o)
	}
	u.showChoices("video-format", "container", videoContainerChoices(o.Codec), "mp4")
	u.modalKey("enter")
	loaded := defaultOptions(root)
	if err := loadSettings(u.config, &loaded); err != nil || loaded.Codec != "h264" || loaded.Container != "mp4" {
		t.Fatal(loaded, err)
	}
}
