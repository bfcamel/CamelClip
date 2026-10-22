// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package ytdlp

import (
	"context"
	"errors"
	"github.com/bfcamel/CamelClip/internal/core"
	"os"
	"path/filepath"
	"testing"
)

func TestPlaylistEnumerationStreamsAndRejectsTruncation(t *testing.T) {
	for _, scenario := range []struct {
		name, script string
		wantError    bool
		count        int
	}{{"complete", "printf '%s\\n' '{\"id\":\"AAAAAAAAAAA\",\"title\":\"one\"}' '{\"id\":\"BBBBBBBBBBB\",\"title\":\"two\"}'", false, 2}, {"empty", "exit 0", false, 0}, {"partial", "printf '%s\\n' '{\"id\":\"AAAAAAAAAAA\"}'; exit 1", true, 0}, {"malformed", "printf '%s\\n' '{broken'", true, 0}} {
		t.Run(scenario.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "yt-dlp")
			os.WriteFile(path, []byte("#!/bin/sh\n"+scenario.script+"\n"), 0700)
			count := 0
			e := New(path).Enumerate(context.Background(), core.PlaylistScan{PlaylistID: "PLfixture00001"}, func(entries []core.PlaylistEntry) error { count += len(entries); return nil })
			if (e != nil) != scenario.wantError || count != scenario.count {
				t.Fatal(e, count)
			}
		})
	}
}
func TestPlaylistEnumerationHonorsConsumerError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "yt-dlp")
	script := "#!/bin/sh\ni=0\nwhile [ $i -lt 300 ]; do echo '{\"id\":\"AAAAAAAAAAA\"}'; i=$((i+1)); done\n"
	os.WriteFile(path, []byte(script), 0700)
	want := errors.New("store failed")
	e := New(path).Enumerate(context.Background(), core.PlaylistScan{PlaylistID: "PLfixture00001"}, func([]core.PlaylistEntry) error { return want })
	if !errors.Is(e, want) {
		t.Fatal(e)
	}
}
