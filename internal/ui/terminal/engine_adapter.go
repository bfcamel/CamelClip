// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package terminal

type tailBuffer struct{ data []byte }

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.data = append(t.data, p...)
	if len(t.data) > 16384 {
		t.data = t.data[len(t.data)-16384:]
	}
	return len(p), nil
}
func (t *tailBuffer) String() string { return string(t.data) }
