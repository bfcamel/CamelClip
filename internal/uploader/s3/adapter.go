// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package s3

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	sdk "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/bfcamel/CamelClip/internal/core"
)

type Adapter struct{}
type Failure struct {
	Code      string
	Temporary bool
}

func (e *Failure) FailureCode() core.ErrorCode {
	switch e.Code {
	case "PreconditionFailed", "ConditionalRequestConflict":
		return core.CodeConflict
	case "RemoteObjectChanged", "RemoteChecksumMismatch", "RemoteSizeMismatch":
		return core.CodeContract
	}
	return core.CodeBackend
}
func (e *Failure) Retryable() bool { return e.Temporary }
func (e *Failure) Error() string   { return "S3: " + e.Code }
func failure(err error) error {
	if err == nil {
		return nil
	}
	var known *Failure
	if errors.As(err, &known) {
		return known
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var status interface{ HTTPStatusCode() int }
	if errors.As(err, &status) && (status.HTTPStatusCode() == 429 || status.HTTPStatusCode() >= 500) {
		return &Failure{Code: fmt.Sprintf("HTTP%d", status.HTTPStatusCode()), Temporary: true}
	}
	var cert *tls.CertificateVerificationError
	var authority x509.UnknownAuthorityError
	if errors.As(err, &cert) || errors.As(err, &authority) {
		return &Failure{Code: "TLSCertificateInvalid"}
	}
	var api smithy.APIError
	if errors.As(err, &api) {
		code := api.ErrorCode()
		allowed := map[string]bool{"AccessDenied": true, "NoSuchBucket": true, "NoSuchKey": true, "NotFound": true, "NoSuchUpload": true, "PreconditionFailed": true, "ConditionalRequestConflict": true, "SlowDown": true, "InternalError": true, "ServiceUnavailable": true, "InvalidAccessKeyId": true, "SignatureDoesNotMatch": true, "NotImplemented": true, "InvalidRequest": true, "ExpiredToken": true, "RequestTimeout": true}
		if !allowed[code] {
			code = "RequestRejected"
		}
		return &Failure{Code: code, Temporary: code == "SlowDown" || code == "InternalError" || code == "ServiceUnavailable" || code == "RequestTimeout"}
	}
	var netErr interface{ Timeout() bool }
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &Failure{Code: "Timeout", Temporary: true}
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return &Failure{Code: "DNSFailure", Temporary: dns.IsTemporary || dns.IsTimeout}
	}
	if errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.EPIPE) || errors.Is(err, io.ErrUnexpectedEOF) {
		return &Failure{Code: "ConnectionFailure", Temporary: true}
	}
	return &Failure{Code: "NetworkOrTLSFailure", Temporary: false}
}
func client(p core.S3Profile, c core.Credentials) *sdk.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	// Limit waiting for response headers, not the duration of a streamed upload.
	transport.ResponseHeaderTimeout = 2 * time.Minute
	if endpoint, e := url.Parse(p.Endpoint); e == nil && endpoint.Scheme == "http" && endpoint.Hostname() == "localhost" {
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			_, port, e := net.SplitHostPort(address)
			if e != nil {
				return nil, e
			}
			return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort("127.0.0.1", port))
		}
	}
	cfg := aws.Config{Region: p.Region, Credentials: credentials.NewStaticCredentialsProvider(c.AccessKey, c.SecretKey, c.SessionToken), HTTPClient: &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, RetryMaxAttempts: 3, RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired, ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired}
	return sdk.NewFromConfig(cfg, func(o *sdk.Options) {
		if p.Endpoint != "" {
			o.BaseEndpoint = aws.String(p.Endpoint)
		}
		o.UsePathStyle = p.Style == "path_style"
	})
}
func (Adapter) Test(ctx context.Context, p core.S3Profile, c core.Credentials, write bool) error {
	if e := core.ValidateProfile(p); e != nil {
		return e
	}
	cl := client(p, c)
	if !write {
		_, e := cl.HeadBucket(ctx, &sdk.HeadBucketInput{Bucket: aws.String(p.Bucket)})
		return failure(e)
	}
	key := path.Join(strings.Trim(p.Prefix, "/"), "camelclip-test-"+core.NewID())
	_, e := cl.PutObject(ctx, &sdk.PutObjectInput{Bucket: aws.String(p.Bucket), Key: aws.String(key), Body: strings.NewReader("CamelClip write check"), IfNoneMatch: aws.String("*")})
	if e != nil {
		return failure(e)
	}
	// Verify conditional writes on the disposable test object before trusting this endpoint.
	_, duplicateErr := cl.PutObject(ctx, &sdk.PutObjectInput{Bucket: aws.String(p.Bucket), Key: aws.String(key), Body: strings.NewReader("must not replace"), IfNoneMatch: aws.String("*")})
	var duplicateAPI smithy.APIError
	conditionalOK := errors.As(duplicateErr, &duplicateAPI) && duplicateAPI.ErrorCode() == "PreconditionFailed"
	_, e = cl.DeleteObject(ctx, &sdk.DeleteObjectInput{Bucket: aws.String(p.Bucket), Key: aws.String(key)})
	if e != nil {
		return fmt.Errorf("test object cleanup failed; remaining key %s: %w", key, failure(e))
	}
	if !conditionalOK {
		return &Failure{Code: "ConditionalWritesUnsupported"}
	}
	return nil
}
func (Adapter) Check(ctx context.Context, p core.S3Profile, c core.Credentials, key string) (core.RemoteObject, error) {
	cl := client(p, c)
	out, e := cl.HeadObject(ctx, &sdk.HeadObjectInput{Bucket: aws.String(p.Bucket), Key: aws.String(key), ChecksumMode: types.ChecksumModeEnabled})
	if e != nil {
		var api smithy.APIError
		if errors.As(e, &api) && (api.ErrorCode() == "NotImplemented" || api.ErrorCode() == "InvalidRequest") {
			out, e = cl.HeadObject(ctx, &sdk.HeadObjectInput{Bucket: aws.String(p.Bucket), Key: aws.String(key)})
		}
	}
	if e != nil {
		return core.RemoteObject{}, failure(e)
	}
	r := core.RemoteObject{Size: aws.ToInt64(out.ContentLength), ETag: aws.ToString(out.ETag), VersionID: aws.ToString(out.VersionId), Verification: "size_verified"}
	if out.ChecksumType == types.ChecksumTypeFullObject {
		r.ChecksumSHA256 = aws.ToString(out.ChecksumSHA256)
	}
	return r, nil
}
func (Adapter) Cleanup(ctx context.Context, p core.S3Profile, c core.Credentials, key, id string) error {
	_, e := client(p, c).AbortMultipartUpload(ctx, &sdk.AbortMultipartUploadInput{Bucket: aws.String(p.Bucket), Key: aws.String(key), UploadId: aws.String(id)})
	if e != nil {
		var api smithy.APIError
		if errors.As(e, &api) && api.ErrorCode() == "NoSuchUpload" {
			return nil
		}
	}
	return failure(e)
}

type counter struct {
	ctx     context.Context
	rate    int64
	start   time.Time
	network atomic.Int64
	mu      sync.Mutex
	read    map[int]int64
	total   int64
	emit    func(core.UploadEvent)
}

func (c *counter) add(part int, pos int64, n int) error {
	sent := c.network.Add(int64(n))
	if c.rate > 0 {
		wait := time.Duration(float64(sent)/float64(c.rate)*float64(time.Second)) - time.Since(c.start)
		if wait > 0 {
			timer := time.NewTimer(wait)
			defer timer.Stop()
			select {
			case <-c.ctx.Done():
				return c.ctx.Err()
			case <-timer.C:
			}
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.read[part] = max(c.read[part], pos)
	bytes := int64(0)
	for _, v := range c.read {
		bytes += v
	}
	if c.emit != nil {
		c.emit(core.UploadEvent{Stage: "uploading", Bytes: min(bytes, c.total), Total: c.total})
	}
	return nil
}

type reader struct {
	r       *io.SectionReader
	counter *counter
	part    int
	pos     int64
}

func (r *reader) Read(p []byte) (int, error) {
	n, e := r.r.Read(p)
	r.pos += int64(n)
	if n > 0 {
		if err := r.counter.add(r.part, r.pos, n); err != nil {
			return n, err
		}
	}
	return n, e
}
func (r *reader) Seek(n int64, whence int) (int64, error) {
	pos, e := r.r.Seek(n, whence)
	r.pos = pos
	return pos, e
}
func (Adapter) Upload(ctx context.Context, p core.S3Profile, c core.Credentials, key string, source core.UploadSource, size int64, sha string, parts int, rate int64, emit func(core.UploadEvent)) (result core.RemoteObject, err error) {
	if e := core.ValidateProfile(p); e != nil {
		return result, e
	}
	if p.Endpoint != "" && !p.ConditionalVerified {
		return result, &Failure{Code: "ConditionalWriteTestRequired"}
	}
	cl := client(p, c)
	defer func() { err = failure(err) }()
	ack := core.RemoteObject{}
	count := &counter{ctx: ctx, rate: rate, start: time.Now(), read: map[int]int64{}, total: size, emit: emit}
	threshold := int64(32 * 1024 * 1024)
	partSize := max(int64(16*1024*1024), (size+9999)/10000)
	parts = max(1, min(parts, 4))
	if size < threshold {
		input := &sdk.PutObjectInput{Bucket: aws.String(p.Bucket), Key: aws.String(key), ContentLength: aws.Int64(size), IfNoneMatch: aws.String("*"), Body: &reader{r: io.NewSectionReader(source, 0, size), counter: count}}
		if p.Provider == "aws" || p.Endpoint == "" {
			digest, e := hex.DecodeString(sha)
			if e != nil {
				return result, e
			}
			input.ChecksumSHA256 = aws.String(base64.StdEncoding.EncodeToString(digest))
		}
		output, e := cl.PutObject(ctx, input)
		err = e
		if err != nil {
			return result, err
		}
		ack.ETag = aws.ToString(output.ETag)
		ack.VersionID = aws.ToString(output.VersionId)
	} else {
		created, e := cl.CreateMultipartUpload(ctx, &sdk.CreateMultipartUploadInput{Bucket: aws.String(p.Bucket), Key: aws.String(key)})
		if e != nil {
			return result, e
		}
		uploadID := aws.ToString(created.UploadId)
		if emit != nil {
			emit(core.UploadEvent{MultipartID: uploadID})
		}
		completed := false
		defer func() {
			if !completed {
				cleanupCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				if cleanupErr := (Adapter{}).Cleanup(cleanupCtx, p, c, key, uploadID); cleanupErr != nil {
					err = fmt.Errorf("multipart cleanup pending: %w", cleanupErr)
				} else if emit != nil {
					emit(core.UploadEvent{Cleanup: true})
				}
			}
		}()
		workCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		var wg sync.WaitGroup
		var mu sync.Mutex
		var first error
		done := make([]types.CompletedPart, (size+partSize-1)/partSize)
		jobs := make(chan int)
		for i := 0; i < parts; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for part := range jobs {
					offset := int64(part) * partSize
					length := min(partSize, size-offset)
					out, e := cl.UploadPart(workCtx, &sdk.UploadPartInput{Bucket: aws.String(p.Bucket), Key: aws.String(key), UploadId: created.UploadId, PartNumber: aws.Int32(int32(part + 1)), ContentLength: aws.Int64(length), Body: &reader{r: io.NewSectionReader(source, offset, length), counter: count, part: part}})
					if e != nil {
						mu.Lock()
						if first == nil {
							first = e
						}
						mu.Unlock()
						cancel()
						return
					}
					done[part] = types.CompletedPart{ETag: out.ETag, PartNumber: aws.Int32(int32(part + 1))}
				}
			}()
		}
	feed:
		for part := range done {
			select {
			case jobs <- part:
			case <-workCtx.Done():
				break feed
			}
		}
		close(jobs)
		wg.Wait()
		if first != nil {
			return result, first
		}
		if e = ctx.Err(); e != nil {
			return result, e
		}
		completedOutput, e := cl.CompleteMultipartUpload(ctx, &sdk.CompleteMultipartUploadInput{Bucket: aws.String(p.Bucket), Key: aws.String(key), UploadId: created.UploadId, MultipartUpload: &types.CompletedMultipartUpload{Parts: done}, IfNoneMatch: aws.String("*")})
		err = e
		if err != nil {
			return result, err
		}
		ack.ETag = aws.ToString(completedOutput.ETag)
		ack.VersionID = aws.ToString(completedOutput.VersionId)
		completed = true
		if emit != nil {
			emit(core.UploadEvent{Cleanup: true})
		}
	}
	if emit != nil {
		emit(core.UploadEvent{Stage: "uploaded", Bytes: size, Total: size, ETag: ack.ETag, VersionID: ack.VersionID})
		emit(core.UploadEvent{Stage: "remote_check", Bytes: size, Total: size})
	}
	result, err = (Adapter{}).Check(ctx, p, c, key)
	if err != nil {
		return result, err
	}
	if ack.VersionID != "" && ack.VersionID != result.VersionID || ack.ETag != "" && ack.ETag != result.ETag {
		return result, &Failure{Code: "RemoteObjectChanged"}
	}
	if result.Size != size {
		return result, &Failure{Code: "RemoteSizeMismatch"}
	}
	if result.ChecksumSHA256 != "" {
		digest, e := hex.DecodeString(sha)
		if e != nil {
			return result, e
		}
		if base64.StdEncoding.EncodeToString(digest) != result.ChecksumSHA256 {
			return result, &Failure{Code: "RemoteChecksumMismatch"}
		}
		result.Verification = "checksum_verified"
	}
	return result, nil
}

var _ core.Uploader = Adapter{}
