package application

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/bfcamel/CamelClip/internal/core"
	"math"
	"strings"
	"sync"
	"testing"
	"time"
)

type memoryClips struct {
	mu         sync.Mutex
	plans      map[string]core.ClipPlan
	batches    map[string]core.ClipBatch
	assets     map[string]core.Asset
	failCommit bool
}

func newMemoryClips() *memoryClips {
	return &memoryClips{plans: map[string]core.ClipPlan{}, batches: map[string]core.ClipBatch{}, assets: map[string]core.Asset{}}
}
func copyClip(b core.ClipBatch) core.ClipBatch {
	data, _ := json.Marshal(b)
	var out core.ClipBatch
	_ = json.Unmarshal(data, &out)
	out.SourcePath = b.SourcePath
	for i := range out.Tasks {
		out.Tasks[i].Candidate = b.Tasks[i].Candidate
	}
	return out
}
func (r *memoryClips) SaveClipPlan(_ context.Context, p core.ClipPlan) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plans[p.ID] = p
	return nil
}
func (r *memoryClips) ClipPlan(_ context.Context, id string) (core.ClipPlan, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.plans[id]
	if !ok {
		return p, errors.New("missing")
	}
	return p, nil
}
func (r *memoryClips) AdmitClip(_ context.Context, b core.ClipBatch) (core.ClipBatch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, old := range r.batches {
		if old.Key == b.Key {
			if old.Plan.Digest != b.Plan.Digest {
				return b, &core.Failure{Code: core.CodeConflict}
			}
			return copyClip(old), nil
		}
	}
	r.batches[b.ID] = copyClip(b)
	return copyClip(b), nil
}
func (r *memoryClips) ClipBatch(_ context.Context, id string) (core.ClipBatch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.batches[id]
	if !ok {
		return b, errors.New("missing")
	}
	return copyClip(b), nil
}
func (r *memoryClips) ClipBatches(_ context.Context, _ int, _ string) (core.ClipBatchPage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := core.ClipBatchPage{}
	for _, b := range r.batches {
		p.Items = append(p.Items, copyClip(b))
	}
	p.Total = len(p.Items)
	return p, nil
}
func (r *memoryClips) NextClip(ctx context.Context, id string) (core.ClipBatch, error) {
	p, _ := r.ClipBatches(ctx, 50, "")
	for _, b := range p.Items {
		if (id == "" || b.ID == id) && !b.Settled() && !b.Paused {
			return b, nil
		}
	}
	return core.ClipBatch{}, errors.New("empty")
}
func (r *memoryClips) SaveClip(_ context.Context, b core.ClipBatch, revision int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.batches[b.ID].Revision != revision {
		return &core.Failure{Code: core.CodeConflict}
	}
	r.batches[b.ID] = copyClip(b)
	return nil
}
func (r *memoryClips) CommitClip(_ context.Context, b core.ClipBatch, revision int64, _ int, a core.Asset) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failCommit {
		r.failCommit = false
		return errors.New("injected commit fault")
	}
	if r.batches[b.ID].Revision != revision {
		return &core.Failure{Code: core.CodeConflict}
	}
	r.batches[b.ID] = copyClip(b)
	r.assets[a.ID] = a
	return nil
}
func (r *memoryClips) ClipDeliveries(context.Context) ([]ClipDelivery, error) { return nil, nil }
func (r *memoryClips) SaveClipDelivery(context.Context, ClipDelivery) error   { return nil }
func (r *memoryClips) ClipProvenance(context.Context, string) ([]ClipProvenance, error) {
	return nil, nil
}
func (r *memoryClips) Assets(context.Context, core.AssetQuery) (core.AssetPage, error) {
	return core.AssetPage{}, nil
}
func (r *memoryClips) Asset(context.Context, string) (core.Asset, error) { return core.Asset{}, nil }
func (r *memoryClips) PutAsset(context.Context, core.Asset) (core.Asset, error) {
	return core.Asset{}, nil
}
func (r *memoryClips) DeleteAssetFile(context.Context, string) error { return nil }
func (r *memoryClips) MarkMissing(context.Context, string, []string, bool) (int, error) {
	return 0, nil
}

type memoryClipFiles struct {
	mu        sync.Mutex
	published int
}

func (f *memoryClipFiles) Acquire(c context.Context, _ string) (context.Context, func(), error) {
	return c, func() {}, nil
}
func (f *memoryClipFiles) CleanupCandidate(context.Context, string) error { return nil }
func (f *memoryClipFiles) Paths(_ context.Context, b, t, out, ext, title string) (string, string, string, error) {
	return "/cache/" + b, "/cache/" + b + "/" + t, out + "/" + t + "." + ext, nil
}
func (f *memoryClipFiles) Publish(context.Context, string, string, core.ClipEvidence) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.published++
	return nil
}
func (f *memoryClipFiles) Validate(context.Context, string, string) error { return nil }
func (f *memoryClipFiles) Cleanup(context.Context, string) error          { return nil }
func (f *memoryClipFiles) Metadata(context.Context, string) (core.Asset, error) {
	return core.Asset{Modified: time.Unix(10, 0)}, nil
}

type memoryClipBackend struct {
	mu                sync.Mutex
	prepares, renders int
	late              func(core.ClipProgress)
}

func (f *memoryClipBackend) Inspect(context.Context, core.ClipRequest, string) (core.ClipSource, error) {
	return core.ClipSource{Identity: "fixture:one", Title: "Fixture", DurationMS: 10000, Video: true, Audio: true}, nil
}
func (f *memoryClipBackend) Prepare(ctx context.Context, p core.ClipPlan, _, _ string, _ func(core.ClipProgress)) (string, core.ClipSource, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prepares++
	s := p.Source
	s.SHA256 = "digest"
	return "/cache/source", s, nil
}
func (f *memoryClipBackend) Render(_ context.Context, _, _ string, _ core.ClipPlan, s core.ClipSegment, observer func(core.ClipProgress)) (core.ClipEvidence, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.renders++
	f.late = observer
	observer(core.ClipProgress{State: core.ClipRendering})
	return core.ClipEvidence{Pipeline: core.ClipPipelineVersion, SHA256: strings.Repeat("a", 64), Width: 160, Height: 90, VideoCodec: "h264", AudioCodec: "aac", Frames: 10, OutputFrames: 10, FirstSourcePTS: float64(s.Range.StartMS) / 1000, LastSourcePTS: float64(s.Range.EndMS)/1000 - 0.1, FirstAudioPTS: float64(s.Range.StartMS) / 1000, LastAudioPTS: float64(s.Range.EndMS)/1000 - 0.001, AudioSamples: 48000, SampleRate: 48000, Size: 10, DurationMS: s.Range.EndMS - s.Range.StartMS}, nil
}
func TestHeadlessClipsSharePreparationRecoverCommitAndFenceLateObservers(t *testing.T) {
	ctx := context.Background()
	r := newMemoryClips()
	q, e := New(ctx, &fakeDownloader{}, &memoryRepository{}, 2)
	if e != nil {
		t.Fatal(e)
	}
	defer q.Close()
	backend := &memoryClipBackend{}
	files := &memoryClipFiles{}
	s, e := NewClips(ctx, r, r, nil, backend, files, q, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	plan, e := s.Plan(ctx, core.ClipRequest{Source: core.ClipSourceRef{URL: "https://example.test/video"}, OutputDir: "/output", Segments: []core.ClipSegment{{Range: core.ClipRange{StartMS: 0, EndMS: 1000}}, {Range: core.ClipRange{StartMS: 1000, EndMS: 2000}}}})
	if e != nil {
		t.Fatal(e)
	}
	b, e := s.Admit(ctx, plan.ID, 1, plan.Digest, "key")
	if e != nil {
		t.Fatal(e)
	}
	same, e := s.Admit(ctx, plan.ID, 1, plan.Digest, "key")
	if e != nil || same.ID != b.ID {
		t.Fatal("replay", e)
	}
	r.mu.Lock()
	r.failCommit = true
	r.mu.Unlock()
	s.Start(b.ID)
	until := time.Now().Add(4 * time.Second)
	for {
		b, e = s.Get(ctx, b.ID)
		if e != nil {
			t.Fatal(e)
		}
		if b.Settled() {
			break
		}
		if time.Now().After(until) {
			t.Fatal("batch did not recover", b, s.Err())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if b.Status() != "completed" {
		t.Fatal(b)
	}
	backend.mu.Lock()
	prepares, renders, late := backend.prepares, backend.renders, backend.late
	backend.mu.Unlock()
	if prepares != 1 || renders != 2 {
		t.Fatal(prepares, renders)
	}
	before := b.Revision
	late(core.ClipProgress{State: core.ClipRendering})
	after, _ := s.Get(ctx, b.ID)
	if after.Revision != before {
		t.Fatal("late callback mutated durable task")
	}
	b.Tasks[0].Segment.Title = "mutated"
	again, _ := s.Get(ctx, b.ID)
	if again.Tasks[0].Segment.Title == "mutated" {
		t.Fatal("mutable snapshot")
	}
	if _, e = s.Control(ctx, b.ID, b.ControlRevision-1, "cancel", ""); core.CodeOf(e) != core.CodeConflict {
		t.Fatal("stale control", e)
	}
}
func TestClipContractRejectsMetricsAndClosesCallbacks(t *testing.T) {
	g := &clipObserver{render: true}
	bad := math.NaN()
	g.observe(core.ClipProgress{State: core.ClipRendering, Metrics: core.Metrics{Percent: &bad}})
	if core.CodeOf(g.close()) != core.CodeContract {
		t.Fatal("invalid metrics accepted")
	}
	called := false
	g = &clipObserver{next: func(core.ClipProgress) { called = true }}
	_ = g.close()
	g.observe(core.ClipProgress{State: core.ClipDownloading})
	if called {
		t.Fatal("closed callback ran")
	}
}
