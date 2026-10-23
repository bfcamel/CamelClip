// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package terminal

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/bfcamel/CamelClip/internal/core"
	"io"
	"os"
)

func runCompanionsCLI(ctx context.Context, o options, config string, args []string) error {
	const help = `Media companions (existing posters and subtitles; no transcription)
  camelclip companions inspect --url URL [--browser firefox]
  camelclip companions inspect --asset ASSET_ID
  camelclip companions poster --id CATALOG_ID
  camelclip companions caption --id CATALOG_ID --track TRACK_ID
  camelclip companions caption --url URL --language en [--browser firefox]
  camelclip companions import --id CATALOG_ID --file FILE --language en
  camelclip companions cues --id SNAPSHOT_ID [--query TEXT] [--cursor OFFSET] [--limit 100]
  camelclip companions range --id SNAPSHOT_ID --first 0 --last 3
  camelclip companions export --id SNAPSHOT_ID --file NEW_FILE.vtt
  camelclip companions links --owner ASSET_OR_CATALOG_ID
  camelclip companions keep --owner ID --id BLOB_ID [--keep=false]
  camelclip companions cache [--prune]
All results are JSON. Caption indices are zero based. Export never overwrites.
Browser-authorized catalogs must be inspected again after restarting the process;
fetch them in the desktop workflow. Saved snapshots remain available offline.
`
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		fmt.Print(help)
		return nil
	}
	set := flag.NewFlagSet("companions", flag.ContinueOnError)
	var url, asset, browser, id, track, query, cursor, file, language, owner string
	var first, last, limit int
	var prune, keep, jsonOutput bool
	set.StringVar(&url, "url", "", "")
	set.StringVar(&asset, "asset", "", "")
	set.StringVar(&browser, "browser", "", "")
	set.StringVar(&id, "id", "", "")
	set.StringVar(&track, "track", "", "")
	set.StringVar(&query, "query", "", "")
	set.StringVar(&cursor, "cursor", "", "")
	set.StringVar(&file, "file", "", "")
	set.StringVar(&language, "language", "und", "")
	set.StringVar(&owner, "owner", "", "")
	set.IntVar(&first, "first", 0, "")
	set.IntVar(&last, "last", 0, "")
	set.IntVar(&limit, "limit", 100, "")
	set.BoolVar(&prune, "prune", false, "")
	set.BoolVar(&keep, "keep", true, "")
	set.BoolVar(&jsonOutput, "json", true, "")
	if e := set.Parse(args[1:]); e != nil {
		return e
	}
	if set.NArg() != 0 {
		return &usageError{"unexpected companion arguments"}
	}
	p, e := openPersistentPool(ctx, o, config, 5)
	if e != nil {
		return e
	}
	defer p.wait()
	if p.workspace == nil || p.workspace.Companions == nil {
		return errors.New("companion service unavailable")
	}
	s := p.workspace.Companions
	if id == "" && (args[0] == "caption" || args[0] == "poster") {
		catalog, err := s.Discover(ctx, core.CompanionRequest{Source: core.ClipSourceRef{URL: url, AssetID: asset}, Browser: browser})
		if err != nil {
			return err
		}
		id = catalog.ID
		if args[0] == "caption" && track == "" {
			for _, t := range catalog.Tracks {
				if t.Language == language {
					track = t.ID
					break
				}
			}
			if track == "" {
				return errors.New("no caption track for selected language")
			}
		}
	}
	var result any
	switch args[0] {
	case "inspect":
		result, e = s.Discover(ctx, core.CompanionRequest{Source: core.ClipSourceRef{URL: url, AssetID: asset}, Browser: browser})
	case "poster":
		result, e = s.Poster(ctx, id)
	case "caption":
		result, e = s.FetchCaption(ctx, id, track)
	case "cues":
		result, e = s.Cues(ctx, id, query, cursor, limit)
	case "range":
		result, e = s.Propose(ctx, id, first, last)
	case "import":
		var f *os.File
		f, e = os.Open(file)
		if e == nil {
			var data []byte
			data, e = io.ReadAll(io.LimitReader(f, 20<<20+1))
			f.Close()
			if e == nil {
				result, e = s.Import(ctx, id, language, data)
			}
		}
	case "export":
		e = s.Export(ctx, id, file)
		result = map[string]bool{"ok": e == nil}
	case "links":
		result, e = s.Links(ctx, owner)
	case "keep":
		e = s.Keep(ctx, owner, id, keep)
		result = map[string]bool{"ok": e == nil}
	case "cache":
		result, e = s.Cache(ctx, prune)
	default:
		return &usageError{"unknown companions command"}
	}
	if e != nil {
		return e
	}
	return emitJSON(result)
}
