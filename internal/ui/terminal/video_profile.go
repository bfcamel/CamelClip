// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package terminal

import "github.com/bfcamel/CamelClip/internal/core"

var videoCodec = func(o options) string { return core.VideoCodec(request(o)) }
var videoContainer = func(o options) string { return core.VideoContainer(request(o)) }
var validateVideoProfile = func(o options) error { return core.ValidateVideoProfile(request(o)) }

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
