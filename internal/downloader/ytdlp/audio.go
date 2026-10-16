// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package ytdlp

import (
	"context"
	"fmt"
	"path/filepath"
)

func processAudio(ctx context.Context, source, dir string, o options, observer pipelineObserver) (string, error) {
	format := o.Audio
	encoder := map[string]string{"mp3": "libmp3lame", "m4a": "aac", "opus": "libopus", "flac": "flac", "wav": "pcm_s16le"}[format]
	if format == "best" {
		info, err := probeMedia(ctx, source, false, o.Duration)
		if err != nil {
			return "", err
		}
		format = "mka"
		encoder = "copy"
		for _, stream := range info.Streams {
			if stream.Type == "audio" {
				if ext, ok := map[string]string{"aac": "m4a", "opus": "opus", "mp3": "mp3", "flac": "flac", "vorbis": "ogg"}[stream.Codec]; ok {
					format = ext
				}
				break
			}
		}
	}
	if encoder == "" {
		return "", fmt.Errorf("unsupported audio format")
	}
	target := filepath.Join(dir, "audio."+format)
	c := command(ctx, "ffmpeg", "-v", "error", "-xerror", "-nostdin", "-y", "-i", source, "-map", "0:a:0", "-vn", "-c:a", encoder, "-progress", "pipe:1", "-nostats", target)
	var tail tailBuffer
	c.Stderr = &tail
	c.Stdout = &ffmpegProgress{duration: o.Duration, stage: "processing", observer: observer}
	if err := c.Run(); err != nil {
		return "", fmt.Errorf("audio conversion failed: %w: %s", err, tail.String())
	}
	return target, nil
}
