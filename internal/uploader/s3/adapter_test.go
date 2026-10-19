// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package s3

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/bfcamel/CamelClip/internal/core"
)

type object struct {
	size int64
	hash string
	etag string
}
type fixtureS3 struct {
	mu                sync.Mutex
	objects           map[string]object
	parts             map[string]map[int]object
	keys              map[string]string
	aborted           int
	ignoreConditional bool
}

func (s *fixtureS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := strings.TrimPrefix(r.URL.Path, "/bucket/")
	query := r.URL.Query()
	bad := func(code int, name string) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(code)
		fmt.Fprintf(w, "<Error><Code>%s</Code><Message>private-secret-must-not-leak</Message></Error>", name)
	}
	if r.Method == "HEAD" {
		if r.URL.Path == "/bucket" || r.URL.Path == "/bucket/" {
			w.WriteHeader(200)
			return
		}
		a, ok := s.objects[key]
		if !ok {
			bad(404, "NoSuchKey")
			return
		}
		w.Header().Set("Content-Length", strconv.FormatInt(a.size, 10))
		etag := a.etag
		if etag == "" {
			etag = "\"fixture-etag\""
		}
		w.Header().Set("ETag", etag)
		return
	}
	if r.Method == "POST" && query.Has("uploads") {
		id := core.NewID()
		s.parts[id] = map[int]object{}
		s.keys[id] = key
		fmt.Fprintf(w, "<InitiateMultipartUploadResult><Bucket>bucket</Bucket><Key>%s</Key><UploadId>%s</UploadId></InitiateMultipartUploadResult>", key, id)
		return
	}
	if r.Method == "DELETE" {
		if id := query.Get("uploadId"); id != "" {
			delete(s.parts, id)
			delete(s.keys, id)
			s.aborted++
		} else {
			delete(s.objects, key)
		}
		w.WriteHeader(204)
		return
	}
	if r.Method == "PUT" {
		h := sha256.New()
		n, _ := io.Copy(h, r.Body)
		o := object{size: n, hash: hex.EncodeToString(h.Sum(nil))}
		if id := query.Get("uploadId"); id != "" {
			part, _ := strconv.Atoi(query.Get("partNumber"))
			s.parts[id][part] = o
			w.Header().Set("ETag", fmt.Sprintf("\"part-%d\"", part))
			return
		}
		if _, exists := s.objects[key]; exists && r.Header.Get("If-None-Match") == "*" && !s.ignoreConditional {
			bad(412, "PreconditionFailed")
			return
		}
		s.objects[key] = o
		w.Header().Set("ETag", "\"fixture-etag\"")
		return
	}
	if r.Method == "POST" && query.Get("uploadId") != "" {
		if _, exists := s.objects[key]; exists && r.Header.Get("If-None-Match") == "*" {
			bad(412, "PreconditionFailed")
			return
		}
		size := int64(0)
		for _, p := range s.parts[query.Get("uploadId")] {
			size += p.size
		}
		s.objects[key] = object{size: size, etag: "\"multipart-etag-3\""}
		delete(s.parts, query.Get("uploadId"))
		fmt.Fprint(w, "<CompleteMultipartUploadResult><ETag>\"multipart-etag-3\"</ETag></CompleteMultipartUploadResult>")
		return
	}
	bad(400, "InvalidRequest")
}
func localS3(t *testing.T) (*fixtureS3, core.S3Profile) {
	t.Helper()
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Skip("loopback unavailable")
	}
	s := &fixtureS3{objects: map[string]object{}, parts: map[string]map[int]object{}, keys: map[string]string{}}
	server := &httptest.Server{Listener: listener, Config: &http.Server{Handler: s}}
	server.Start()
	t.Cleanup(server.Close)
	return s, core.S3Profile{ID: core.NewID(), Name: "Test", Endpoint: server.URL, Region: "us-east-1", Bucket: "bucket", Style: "path_style", Enabled: true, AllowLoopbackHTTP: true, ConditionalVerified: true}
}
func uploadFixture(t *testing.T, size int64) (*os.File, string) {
	t.Helper()
	f, e := os.Create(filepath.Join(t.TempDir(), "source"))
	if e != nil {
		t.Fatal(e)
	}
	if e = f.Truncate(size); e != nil {
		t.Fatal(e)
	}
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		t.Fatal(e)
	}
	sum := hex.EncodeToString(h.Sum(nil))
	f.Seek(0, io.SeekStart)
	t.Cleanup(func() { f.Close() })
	return f, sum
}
func TestUploadSingleMultipartAndConditionalPublication(t *testing.T) {
	s, p := localS3(t)
	credentials := core.Credentials{AccessKey: "test-access", SecretKey: "test-secret"}
	adapter := Adapter{}
	if e := adapter.Test(context.Background(), p, credentials, false); e != nil {
		t.Fatal(e)
	}
	if e := adapter.Test(context.Background(), p, credentials, true); e != nil {
		t.Fatal(e)
	}
	for _, size := range []int64{1024, 40 << 20} {
		source, sum := uploadFixture(t, size)
		key := fmt.Sprintf("file-%d", size)
		measured := false
		result, e := adapter.Upload(context.Background(), p, credentials, key, source, size, sum, 2, 0, func(event core.UploadEvent) {
			if event.Bytes > 0 {
				measured = true
			}
		})
		if e != nil {
			t.Fatal(e)
		}
		if !measured || result.Size != size || result.Verification != "size_verified" {
			t.Fatal(result)
		}
		if _, e = adapter.Upload(context.Background(), p, credentials, key, source, size, sum, 2, 0, nil); e == nil || !strings.Contains(e.Error(), "PreconditionFailed") {
			t.Fatal("existing object overwritten", e)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.parts) != 0 {
		t.Fatal("orphaned multipart")
	}
}
func TestWriteProbeRejectsIgnoredConditionals(t *testing.T) {
	s, p := localS3(t)
	s.ignoreConditional = true
	e := (Adapter{}).Test(context.Background(), p, core.Credentials{AccessKey: "test", SecretKey: "test"}, true)
	if e == nil || !strings.Contains(e.Error(), "ConditionalWritesUnsupported") {
		t.Fatal(e)
	}
	if strings.Contains(e.Error(), "private-secret") {
		t.Fatal("raw provider error exposed")
	}
}
