// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package terminal

import (
	"context"
	"encoding/json"
	"github.com/bfcamel/CamelClip/internal/application"
	"github.com/bfcamel/CamelClip/internal/bootstrap"
	"io"
	"os"
	"testing"
)

func TestQueueActionsReportPartialFailureAfterCompleteJSON(t *testing.T) {
	ctx := context.Background()
	w, err := bootstrap.OpenWorkspace(bootstrap.Downloader())(ctx, t.TempDir()+"/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	p := &jobPool{workspace: w, service: w.Queue}
	o := testOptions(t.TempDir())
	run := func(pos []string) ([]application.ActionResult, error) {
		t.Helper()
		old := os.Stdout
		r, writer, e := os.Pipe()
		if e != nil {
			t.Fatal(e)
		}
		os.Stdout = writer
		defer func() { os.Stdout = old; r.Close() }()
		actionErr := queueCLI(ctx, p, pos, catalogFlags{}, o)
		writer.Close()
		os.Stdout = old
		data, e := io.ReadAll(r)
		if e != nil {
			t.Fatal(e)
		}
		var results []application.ActionResult
		if e = json.Unmarshal(data, &results); e != nil {
			t.Fatalf("invalid JSON %q: %v", data, e)
		}
		return results, actionErr
	}
	results, err := run([]string{"add", "https://example.com/valid", "not-a-url"})
	if err == nil || len(results) != 2 || results[0].ID == 0 || results[0].Error != "" || results[1].Error == "" {
		t.Fatal(results, err)
	}
	results, err = run([]string{"cancel", "1,999999"})
	if err == nil || len(results) != 2 || results[0].Error != "" || results[1].Error == "" {
		t.Fatal(results, err)
	}
	results, err = run([]string{"add", "https://example.com/another"})
	if err != nil || len(results) != 1 || results[0].Error != "" {
		t.Fatal(results, err)
	}
}
