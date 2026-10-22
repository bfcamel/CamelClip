// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package core

import "testing"

func TestPlaylistCanonicalIdentityAndGeneratedListRejection(t *testing.T) {
	for _, v := range []string{"PLfixture00001", "https://www.youtube.com/playlist?list=PLfixture00001", "https://www.youtube.com/watch?v=AAAAAAAAAAA&list=PLfixture00001"} {
		id, e := PlaylistID(v)
		if e != nil || id != "PLfixture00001" {
			t.Fatal(id, e)
		}
	}
	for _, v := range []string{"https://example.com/playlist?list=PLfixture00001", "https://youtube.com.evil/playlist?list=PLfixture00001", "RDabcdefghijk", "WL", "https://user:password@youtube.com/playlist?list=PLfixture00001"} {
		if _, e := PlaylistID(v); e == nil {
			t.Fatal(v)
		}
	}
}
