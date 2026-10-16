// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package architecture

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestPackageDependencyDirection(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	prefix := "github.com/bfcamel/video-downloader/internal/"
	rules := map[string][]string{"core": {}, "application": {"core"}, "config": {"core"}, "downloader/ytdlp": {"core"}, "storage/sqlite": {"application", "core"}, "storage/jsonconfig": {"config"}, "ui/terminal": {"application", "core", "config", "diagnostics"}, "diagnostics": {"config"}, "security": {"core"}, "library": {"core"}, "systemmetrics": {"core"}, "uploader/s3": {"core"}}
	for dir, allowed := range rules {
		err := filepath.WalkDir(filepath.Join(root, "internal", dir), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			for _, item := range f.Imports {
				value, _ := strconv.Unquote(item.Path.Value)
				if strings.HasPrefix(value, prefix) {
					target := strings.TrimPrefix(value, prefix)
					ok := false
					for _, a := range allowed {
						if target == a {
							ok = true
						}
					}
					if !ok {
						t.Errorf("%s imports forbidden dependency %s", dir, target)
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
