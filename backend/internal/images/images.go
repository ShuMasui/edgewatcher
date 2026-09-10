// Package images owns everything about the image bucket that is not an S3
// API call itself: where an observation's bytes live (the key layout of
// docs/05-backend.md §1.4), how the web client is handed a readable URL for
// them (§1.5), and how device-api writes them (§1.3).
//
// It is deliberately split across three tiny types rather than one S3
// client wrapper, because the three functions have three different IAM
// ceilings (§2.4) and are never all available at once: device-api may
// PutObject and nothing else, web-api may sign GetObject and nothing else,
// and the key layout is shared by both. Handing every caller one fat type
// would put a method on it that its own execution role cannot perform —
// a runtime AccessDenied where a compile error belongs.
package images

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/ShuMasui/edgewatcher/backend/internal/ids"
)

const (
	// keyPrefix leads every object. deviceId comes first so a device's
	// images can be listed and deleted by prefix (docs/05-backend.md §1.4,
	// DATA-02) without touching any other device's.
	keyPrefix = "observations/"

	// thumbSuffix distinguishes the thumbnail from the full image within
	// the SAME prefix. Splitting them into separate trees would mean a
	// prefix delete has two places to visit instead of one.
	thumbSuffix = "_thumb"

	imageExt = ".jpg"

	// contentTypeJPEG is stored on the object because S3 echoes it back on
	// GET, including through a presigned URL — the browser's <img> relies
	// on it.
	contentTypeJPEG = "image/jpeg"

	// dateLayout is the date segment's format: the human-readable grouping
	// a person browsing the console navigates by.
	dateLayout = "2006-01-02"
)

// ObservationKeys returns the S3 keys for an observation's full image and
// its thumbnail (docs/05-backend.md §1.4):
//
//	observations/<deviceId>/<YYYY-MM-DD>/<observationId>.jpg
//	observations/<deviceId>/<YYYY-MM-DD>/<observationId>_thumb.jpg
//
// The date segment is derived from capturedAt in JST, not UTC. It exists so
// a human can navigate the bucket by the day a photo was taken in Japan;
// deriving it in UTC would file every photo taken after 09:00 JST — which is
// most of them — under the previous day. capturedAt is normalized into
// ids.JST first, so an instant expressed as "+09:00" and the same instant
// expressed as "Z" produce the same key. Without that normalization the key
// would depend on the location field of the caller's time.Time, and an
// upload's key could differ from the key a later read derives for the same
// observation.
//
// observationId is the ULID that is also the DynamoDB SK, which is what
// makes an orphaned object (S3 written, DynamoDB not — the deliberate
// failure direction of §1.3) traceable back to the item that should exist.
func ObservationKeys(deviceID, observationID string, capturedAt time.Time) (imageKey, thumbnailKey string) {
	day := capturedAt.In(ids.JST).Format(dateLayout)
	base := keyPrefix + deviceID + "/" + day + "/" + observationID
	return base + imageExt, base + thumbSuffix + imageExt
}

// PresignAPI is the subset of *s3.PresignClient that Signer calls.
type PresignAPI interface {
	PresignGetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error)
}

// SignedURL is a presigned GET and the instant it stops working.
//
// ExpiresAt is returned alongside the URL rather than left for the caller to
// recompute because GET /observations/{id}/image must report it in its body
// (web/src/types/api.ts, GetObservationImageResponse), and a caller
// recomputing it from its own time.Now() would drift from the value actually
// signed into the URL.
type SignedURL struct {
	URL       string
	ExpiresAt int64
}

// Signer issues presigned GET URLs for objects in the image bucket. It is
// the only way image bytes reach the web client: the bucket stays private
// (docs/05-backend.md §1.5).
//
// Signing is pure computation — no S3 request is made — so a handler may
// sign a whole page of thumbnails without any per-object latency
// (docs/03-web.md §3.6). The signature does, however, inherit the signer's
// permissions, which is why web-api's role carries s3:GetObject even though
// it never calls GetObject itself (§2.4).
type Signer struct {
	presign PresignAPI
	bucket  string
	ttl     time.Duration
}

// NewSigner constructs a Signer. ttl is the URL's validity window
// (SIGNED_URL_TTL, default 900s per docs/05-backend.md §3.6).
func NewSigner(presign PresignAPI, bucket string, ttl time.Duration) *Signer {
	return &Signer{presign: presign, bucket: bucket, ttl: ttl}
}

// SignGetObject returns a presigned GET for key, valid for the Signer's TTL.
//
// now is passed in rather than read from the clock so that every URL in one
// response reports the same expiry as the rest of that response. The value
// only sets the reported ExpiresAt; the signature's own validity window is
// anchored by the SDK to the real signing time, so a caller cannot backdate
// a URL into a longer life by handing us an old `now`.
//
// The returned URL is a credential in its own right for the length of the
// TTL. It must never be logged (docs/05-backend.md §2.5) — logging one is
// equivalent to leaking the image for the log's retention period, not for
// the URL's.
func (s *Signer) SignGetObject(ctx context.Context, key string, now time.Time) (SignedURL, error) {
	req, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(s.ttl))
	if err != nil {
		return SignedURL{}, fmt.Errorf("images: presign %q: %w", key, err)
	}
	return SignedURL{URL: req.URL, ExpiresAt: now.Add(s.ttl).Unix()}, nil
}

// PutObjectAPI is the subset of *s3.Client that Uploader calls. It is
// exactly one method wide because device-api's execution role grants
// exactly one S3 action (docs/05-backend.md §2.4).
type PutObjectAPI interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

// Uploader writes observation images into the bucket. Only device-api holds
// one.
type Uploader struct {
	client PutObjectAPI
	bucket string
}

// NewUploader constructs an Uploader over the image bucket.
func NewUploader(client PutObjectAPI, bucket string) *Uploader {
	return &Uploader{client: client, bucket: bucket}
}

// PutJPEG writes body to key.
//
// Re-uploading the same observation overwrites the same key rather than
// accumulating a second object, because the key is derived from the
// device-assigned observationId (docs/05-backend.md §2.6). That is what
// makes a retried upload idempotent in S3 as well as in DynamoDB.
//
// body is already fully in memory — it arrived as part of a Lambda
// invocation payload, capped at 6MB (§1.3) — so wrapping it in a
// bytes.Reader costs nothing and lets the SDK compute a checksum without
// buffering it again.
func (u *Uploader) PutJPEG(ctx context.Context, key string, body []byte) error {
	_, err := u.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(u.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(body),
		ContentType: aws.String(contentTypeJPEG),
	})
	if err != nil {
		return fmt.Errorf("images: put %q: %w", key, err)
	}
	return nil
}
