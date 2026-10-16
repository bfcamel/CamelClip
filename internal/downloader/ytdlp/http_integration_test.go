// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package ytdlp

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
	"time"

	"github.com/bfcamel/video-downloader/internal/core"
)

// Exercise actual yt-dlp selectors, manifest output and numerical progress,
// without contacting YouTube or reading browser credentials.
func TestRealDownloaderLocalHTTP(t *testing.T) {
	executable, err := exec.LookPath("yt-dlp")
	if err != nil {
		t.Skip("yt-dlp unavailable")
	}
	fixture := makeFixture(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("local HTTP listener unavailable: %v", err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		http.ServeFile(w, r, fixture)
	}))
	server.Listener.Close()
	server.Listener = listener
	server.Start()
	defer server.Close()
	for _, mode := range []string{"video", "audio"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			r := defaults()
			r.URL, r.Out, r.Mode, r.ExpectedParts = server.URL+"/fixture.mp4", t.TempDir(), mode, 1
			r.Cookies, r.Duration, r.Quality = "", 1, "best"
			measured := false
			result, err := New(executable).Download(ctx, r, func(e core.Event) {
				if e.Stage == core.Downloading && e.Metrics != nil && e.Metrics.DownloadedBytes > 0 {
					measured = true
				}
			})
			if err != nil {
				t.Fatal(err)
			}
			if !measured || result.Path == "" {
				t.Fatalf("missing numerical progress or publication: %+v", result)
			}
			if err := verifyMedia(ctx, result.Path, mode == "video", 1); err != nil {
				t.Fatal(err)
			}
			if mode == "audio" {
				info, err := probeMedia(ctx, result.Path, false, 1)
				if err != nil {
					t.Fatal(err)
				}
				for _, stream := range info.Streams {
					if stream.Type == "video" || stream.Type == "audio" && stream.Codec != "mp3" {
						t.Fatalf("audio output ignored selected profile: %+v", info)
					}
				}
			}
		})
	}
}
