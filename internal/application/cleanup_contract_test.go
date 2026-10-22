package application_test

import (
	"context"
	"errors"
	"github.com/bfcamel/CamelClip/internal/application"
	"github.com/bfcamel/CamelClip/internal/core"
	"github.com/bfcamel/CamelClip/internal/security"
	"github.com/bfcamel/CamelClip/internal/storage/sqlite"
	"testing"
)

type auditCleanupBackend struct {
	fakeCloud
	entered, release chan struct{}
}

func (b *auditCleanupBackend) Cleanup(context.Context, core.S3Profile, core.Credentials, string, string) error {
	close(b.entered)
	<-b.release
	return nil
}
func TestCleanupPreservesConcurrentCancel(t *testing.T) {
	ctx := context.Background()
	r, e := sqlite.Open(ctx, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	b := &auditCleanupBackend{fakeCloud: fakeCloud{remote: map[string]core.RemoteObject{}}, entered: make(chan struct{}), release: make(chan struct{})}
	factory := func() core.CredentialVault { return security.New() }
	c := application.NewCloud(ctx, r, r, r, factory(), factory, b)
	defer c.Close()
	c.Restrict([]string{})
	if e = c.Initialize(ctx, []byte("audit master password")); e != nil {
		t.Fatal(e)
	}
	p := core.S3Profile{ID: core.NewID(), Name: "audit", Region: "auto", Bucket: "bucket", Style: "path_style", Enabled: true}
	if e = c.SaveProfile(ctx, p, &core.Credentials{AccessKey: "access", SecretKey: "private-key"}); e != nil {
		t.Fatal(e)
	}
	j, e := r.EnqueueUpload(ctx, core.UploadJob{ID: core.NewID(), ProfileID: p.ID, Profile: p, AssetID: "asset", Key: "key", State: "queued", MultipartID: "old-multipart"})
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { done <- c.Cleanup(ctx, j.ID) }()
	<-b.entered
	if e = c.Cancel(ctx, j.ID); e != nil {
		t.Fatal(e)
	}
	canceled, _ := c.Get(ctx, j.ID)
	if canceled.State != "canceled" {
		t.Fatal(canceled.State)
	}
	close(b.release)
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	got, _ := c.Get(ctx, j.ID)
	if got.State != "canceled" {
		t.Fatal("observation changed", got.State)
	}
	if got.MultipartID != "" {
		t.Fatal("cleanup checkpoint not cleared")
	}
}

type failedRecoveryRepository struct {
	*sqlite.Repository
	failure error
}

func (r failedRecoveryRepository) RecoverUploads(context.Context) error { return r.failure }
func TestCloudFatalFailurePropagatesThroughReadsAndClose(t *testing.T) {
	ctx := context.Background()
	r, e := sqlite.Open(ctx, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	failure := errors.New("injected recovery failure")
	factory := func() core.CredentialVault { return security.New() }
	c := application.NewCloud(ctx, r, failedRecoveryRepository{r, failure}, r, factory(), factory, &fakeCloud{})
	if _, e = c.List(ctx, 10, 0); !errors.Is(e, failure) {
		t.Fatal(e)
	}
	if _, e = c.Get(ctx, "job"); !errors.Is(e, failure) {
		t.Fatal(e)
	}
	if e = c.Close(); !errors.Is(e, failure) {
		t.Fatal(e)
	}
	if e = c.Close(); !errors.Is(e, failure) {
		t.Fatal(e)
	}
	if e = c.SetOptions(core.UploadOptions{Objects: 2, Parts: 2}); !errors.Is(e, application.ErrClosed) {
		t.Fatal(e)
	}
	if e = c.Unlock(ctx, []byte("audit master password")); !errors.Is(e, application.ErrClosed) {
		t.Fatal(e)
	}
}
