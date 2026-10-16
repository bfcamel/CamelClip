// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package terminal

import (
	"strings"
	"testing"
)

func TestAuthenticationDiagnosticsAndProfiles(t *testing.T) {
	raw := "ERROR: Sign in to confirm you're not a bot"
	got := explain(raw)
	if !strings.Contains(got, "не бот") || !strings.Contains(got, raw) {
		t.Fatal(got)
	}
	for _, browser := range []string{"firefox", "firefox:default-release", "firefox:/tmp/my profile::Personal", "chrome:Default"} {
		o := defaults()
		o.Cookies = browser
		if err := validate(o); err != nil {
			t.Fatal(browser, err)
		}
	}
	o := defaults()
	o.Cookies = "unknown:profile"
	if validate(o) == nil {
		t.Fatal("unknown browser accepted")
	}
}
