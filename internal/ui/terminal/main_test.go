// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package terminal

import (
	"testing"
)

func TestExpandHome(t *testing.T) {
	for input, expected := range map[string]string{"~": "/Users/test", "~/My Videos": "/Users/test/My Videos", "/tmp/video": "/tmp/video", "relative": "relative"} {
		if actual := expandHome(input, "/Users/test"); actual != expected {
			t.Fatalf("%q: got %q", input, actual)
		}
	}
}

func defaults() options {
	return options{URL: "https://example.com/watch?v=123&x=$(touch nope)", Mode: "video", Quality: "1080", Audio: "mp3", Out: "/tmp/My Downloads", runtime: &testDependencies}
}
