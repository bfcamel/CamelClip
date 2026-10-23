package sqlite

import (
	"context"
	"encoding/json"
	"github.com/bfcamel/CamelClip/internal/application"
	"github.com/bfcamel/CamelClip/internal/core"
	"path/filepath"
	"testing"
	"time"
)

func storedClip() core.ClipBatch {
	r := core.ClipRequest{Source: core.ClipSourceRef{URL: "https://example.test/video"}, Mode: "video", Audio: "m4a", Quality: "best", Accuracy: "precise", OutputDir: "/output", Segments: []core.ClipSegment{{Title: "One", Range: core.ClipRange{StartMS: 1000, EndMS: 2000}}}}
	return core.ClipBatch{ID: core.NewID(), Key: "one", Revision: 1, ControlRevision: 1, Plan: core.ClipPlan{ID: core.NewID(), Digest: "reviewed", Request: r, Source: core.ClipSource{Identity: "fixture:1", DurationMS: 10000, Video: true, Audio: true}, Expires: time.Now().Add(time.Hour)}, Tasks: []core.ClipTask{{ID: core.NewID(), Ordinal: 0, Segment: r.Segments[0], State: core.ClipWaitingSource}}}
}
func TestClipAdmissionReplayCASAndPrivateReferences(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	r, e := Open(ctx, dir)
	if e != nil {
		t.Fatal(e)
	}
	b := storedClip()
	if b, e = r.AdmitClip(ctx, b); e != nil {
		t.Fatal(e)
	}
	other := b
	other.ID = core.NewID()
	same, e := r.AdmitClip(ctx, other)
	if e != nil || same.ID != b.ID {
		t.Fatal("replay", e)
	}
	other.Plan.Digest = "changed"
	if _, e = r.AdmitClip(ctx, other); core.CodeOf(e) != core.CodeConflict {
		t.Fatal("changed replay", e)
	}
	b.SourcePath = "/private/cache/source"
	b.Tasks[0].Candidate = "/private/staging/candidate"
	b.Revision++
	if e = r.SaveClip(ctx, b, 1); e != nil {
		t.Fatal(e)
	}
	if e = r.SaveClip(ctx, b, 1); core.CodeOf(e) != core.CodeConflict {
		t.Fatal("stale save", e)
	}
	r.Close()
	r, e = Open(ctx, dir)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	b, e = r.ClipBatch(ctx, b.ID)
	if e != nil || b.SourcePath != "/private/cache/source" || b.Tasks[0].Candidate != "/private/staging/candidate" {
		t.Fatal("private references lost", e)
	}
	data, _ := json.Marshal(b)
	var pub map[string]any
	_ = json.Unmarshal(data, &pub)
	if _, ok := pub["source_path"]; ok {
		t.Fatal("private source exposed")
	}
	if _, e = r.ClipBatches(ctx, 50, "1garbage"); e == nil {
		t.Fatal("invalid cursor")
	}
}
func TestClipCommitRollsBackAssetProvenanceAndDeliveryTogether(t *testing.T) {
	ctx := context.Background()
	r, e := Open(ctx, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	b := storedClip()
	profile := core.S3Profile{ID: "selected", Name: "Selected", Region: "us-east-1", Bucket: "bucket", Style: "path_style", Enabled: true}
	b.Plan.Destinations = []core.S3Profile{profile}
	if _, e = r.AdmitClip(ctx, b); e != nil {
		t.Fatal(e)
	}
	ev := core.ClipEvidence{SHA256: "digest", Size: 10, DurationMS: 1000}
	b.Tasks[0].State = core.ClipDone
	b.Tasks[0].Evidence = &ev
	b.Tasks[0].AssetID = "asset"
	b.Tasks[0].FinalPath = "/output/clip.mp4"
	b.Revision = 2
	a := core.Asset{ID: "asset", Path: b.Tasks[0].FinalPath, Status: "available"}
	if _, e = r.db.Exec(`CREATE TRIGGER fail_clip BEFORE INSERT ON clip_provenance BEGIN SELECT RAISE(ABORT,'injected'); END`); e != nil {
		t.Fatal(e)
	}
	if e = r.CommitClip(ctx, b, 1, 0, a); e == nil {
		t.Fatal("fault accepted")
	}
	var count int
	_ = r.db.QueryRow("SELECT count(*) FROM assets").Scan(&count)
	if count != 0 {
		t.Fatal("asset escaped rollback")
	}
	got, _ := r.ClipBatch(ctx, b.ID)
	if got.Revision != 1 {
		t.Fatal("batch escaped rollback")
	}
	r.db.Exec("DROP TRIGGER fail_clip")
	if e = r.CommitClip(ctx, b, 1, 0, a); e != nil {
		t.Fatal(e)
	}
	deliveries, e := r.ClipDeliveries(ctx)
	if e != nil || len(deliveries) != 1 {
		t.Fatal(deliveries, e)
	}
	prov, e := r.ClipProvenance(ctx, "asset")
	if e != nil || len(prov) != 1 {
		t.Fatal(prov, e)
	}
	if e = r.CommitClip(ctx, b, 1, 0, a); core.CodeOf(e) != core.CodeConflict {
		t.Fatal("stale commit accepted", e)
	}
}
func TestSchemaFiveUpgradePreservesQueueCatalogAndOpaqueVault(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	r, e := Open(ctx, dir)
	if e != nil {
		t.Fatal(e)
	}
	if e = r.Save(ctx, application.Snapshot{NextID: 7, Jobs: []application.Job{job(7)}}); e != nil {
		t.Fatal(e)
	}
	asset, e := r.PutAsset(ctx, core.Asset{Path: filepath.Join(dir, "existing.mp4"), Title: "Existing", Status: "available"})
	if e != nil {
		t.Fatal(e)
	}
	vault := "opaque-test-vault-record"
	if _, e = r.db.Exec("INSERT INTO s3_secret_store(id,record) VALUES(1,?)", vault); e != nil {
		t.Fatal(e)
	}
	for _, q := range []string{"DROP TABLE clip_plans", "DROP TABLE clip_batches", "DROP TABLE clip_provenance", "DROP TABLE clip_deliveries", "PRAGMA user_version=5"} {
		if _, e = r.db.Exec(q); e != nil {
			t.Fatal(e)
		}
	}
	r.Close()
	r, e = Open(ctx, dir)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	snapshot, e := r.Load(ctx)
	if e != nil || snapshot.NextID != 7 || len(snapshot.Jobs) != 1 {
		t.Fatal(snapshot, e)
	}
	saved, e := r.Asset(ctx, asset.ID)
	if e != nil || saved.Title != asset.Title {
		t.Fatal(saved, e)
	}
	var record string
	if e = r.db.QueryRow("SELECT record FROM s3_secret_store").Scan(&record); e != nil || record != vault {
		t.Fatal("vault changed", e)
	}
	if _, e = r.ClipBatches(ctx, 50, ""); e != nil {
		t.Fatal("clip tables not created", e)
	}
}
