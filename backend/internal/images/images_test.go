package images

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// mustParse is RFC3339 or bust — a malformed literal in a test fixture is a
// bug in the test, not a condition to handle.
func mustParse(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("bad fixture timestamp %q: %v", s, err)
	}
	return ts
}

// TestObservationKeys_Layout pins the exact key shape docs/05-backend.md §1.4
// specifies. The deviceId-first prefix is what makes a device's images
// deletable by prefix (DATA-02), and the shared prefix with a _thumb suffix
// is what keeps that a single prefix rather than two trees.
func TestObservationKeys_Layout(t *testing.T) {
	capturedAt := mustParse(t, "2026-09-08T02:04:05Z") // 11:04 JST, same day
	img, thumb := ObservationKeys("dev-1", "01J0OBS", capturedAt)

	if want := "observations/dev-1/2026-09-08/01J0OBS.jpg"; img != want {
		t.Errorf("image key = %q, want %q", img, want)
	}
	if want := "observations/dev-1/2026-09-08/01J0OBS_thumb.jpg"; thumb != want {
		t.Errorf("thumbnail key = %q, want %q", thumb, want)
	}
}

// TestObservationKeys_DateIsJST is the reason ObservationKeys takes a
// time.Time rather than a preformatted date string. The date segment exists
// so a human browsing the console sees photos grouped by the day they were
// taken in Japan (docs/05-backend.md §1.4); deriving it in UTC would file
// every evening photo under the previous day. 15:30Z is 00:30 JST the NEXT
// day, so a UTC derivation and a JST one disagree here by one day.
func TestObservationKeys_DateIsJST(t *testing.T) {
	capturedAt := mustParse(t, "2026-09-08T15:30:00Z")
	img, thumb := ObservationKeys("dev-1", "01J0OBS", capturedAt)

	if !strings.Contains(img, "/2026-09-09/") {
		t.Errorf("image key = %q, want the JST date 2026-09-09", img)
	}
	if !strings.Contains(thumb, "/2026-09-09/") {
		t.Errorf("thumbnail key = %q, want the JST date 2026-09-09", thumb)
	}
}

// TestObservationKeys_AlreadyJSTInputAgrees guards the normalization itself.
// A caller handing us a time.Time already in +09:00 must land on the same
// key as the equivalent instant expressed in UTC — otherwise the key an
// upload writes and the key a later read derives could differ purely by the
// location field of a time.Time, which no test of a single call would catch.
func TestObservationKeys_AlreadyJSTInputAgrees(t *testing.T) {
	utc := mustParse(t, "2026-09-08T15:30:00Z")
	jst := mustParse(t, "2026-09-09T00:30:00+09:00")

	gotUTC, _ := ObservationKeys("dev-1", "01J0OBS", utc)
	gotJST, _ := ObservationKeys("dev-1", "01J0OBS", jst)
	if gotUTC != gotJST {
		t.Errorf("same instant produced different keys: %q vs %q", gotUTC, gotJST)
	}
}

// newTestSigner builds a Signer over a presign client with static dummy
// credentials. Presigning is pure computation — no S3 call is made — so this
// exercises the real signing path without any network or account.
func newTestSigner(t *testing.T, ttl time.Duration) *Signer {
	t.Helper()
	client := s3.NewFromConfig(aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("AKIAEXAMPLE", "secret", ""),
	})
	return NewSigner(s3.NewPresignClient(client), "ew-images-dev", ttl)
}

// TestSigner_SignGetObject_Expiry pins the 15-minute TTL from
// docs/05-backend.md §1.5 onto the wire, where it is actually enforced,
// rather than onto a Go field nobody downstream reads.
func TestSigner_SignGetObject_Expiry(t *testing.T) {
	s := newTestSigner(t, 900*time.Second)
	now := mustParse(t, "2026-09-08T02:00:00Z")

	signed, err := s.SignGetObject(context.Background(), "observations/dev-1/2026-09-08/01J0OBS.jpg", now)
	if err != nil {
		t.Fatalf("SignGetObject: %v", err)
	}

	u, err := url.Parse(signed.URL)
	if err != nil {
		t.Fatalf("signed URL is not a URL: %v", err)
	}
	if got := u.Query().Get("X-Amz-Expires"); got != "900" {
		t.Errorf("X-Amz-Expires = %q, want %q", got, "900")
	}
	if !strings.Contains(u.Host, "ew-images-dev") && !strings.Contains(u.Path, "ew-images-dev") {
		t.Errorf("signed URL %q addresses neither the bucket host nor path", u.String())
	}
	if !strings.Contains(u.Path, "01J0OBS.jpg") {
		t.Errorf("signed URL path = %q, want the object key in it", u.Path)
	}
}

// TestSigner_SignGetObject_ExpiresAtIsDerivedFromNow makes the returned
// expiry the caller's clock plus the TTL, not time.Now(). The web client
// shows "this link stops working at ..." from this value, and a handler
// that already fixed `now` for the rest of its response must not report a
// different instant here.
func TestSigner_SignGetObject_ExpiresAtIsDerivedFromNow(t *testing.T) {
	s := newTestSigner(t, 900*time.Second)
	now := mustParse(t, "2026-09-08T02:00:00Z")

	signed, err := s.SignGetObject(context.Background(), "observations/dev-1/2026-09-08/01J0OBS.jpg", now)
	if err != nil {
		t.Fatalf("SignGetObject: %v", err)
	}
	if want := now.Add(900 * time.Second).Unix(); signed.ExpiresAt != want {
		t.Errorf("ExpiresAt = %d, want %d", signed.ExpiresAt, want)
	}
}

// TestSigner_NonDefaultTTLReachesTheWire guards against the TTL being read
// from config but then hardcoded at the call site — a mistake that the
// 900-second test above could not distinguish from correct behaviour,
// because 900 is also the default.
func TestSigner_NonDefaultTTLReachesTheWire(t *testing.T) {
	s := newTestSigner(t, 60*time.Second)
	now := mustParse(t, "2026-09-08T02:00:00Z")

	signed, err := s.SignGetObject(context.Background(), "observations/dev-1/2026-09-08/01J0OBS.jpg", now)
	if err != nil {
		t.Fatalf("SignGetObject: %v", err)
	}
	u, err := url.Parse(signed.URL)
	if err != nil {
		t.Fatalf("signed URL is not a URL: %v", err)
	}
	if got := u.Query().Get("X-Amz-Expires"); got != "60" {
		t.Errorf("X-Amz-Expires = %q, want %q", got, "60")
	}
	if want := now.Add(60 * time.Second).Unix(); signed.ExpiresAt != want {
		t.Errorf("ExpiresAt = %d, want %d", signed.ExpiresAt, want)
	}
}

// fakePutter records what Uploader hands the SDK.
type fakePutter struct {
	calls []*s3.PutObjectInput
	err   error
}

func (f *fakePutter) PutObject(ctx context.Context, in *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	f.calls = append(f.calls, in)
	if f.err != nil {
		return nil, f.err
	}
	return &s3.PutObjectOutput{}, nil
}

// TestUploader_PutJPEG pins bucket, key, body and content type. Content type
// matters beyond tidiness: the browser loads these bytes through an <img>
// from a presigned URL, and S3 echoes back whatever was stored here.
func TestUploader_PutJPEG(t *testing.T) {
	putter := &fakePutter{}
	u := NewUploader(putter, "ew-images-dev")

	body := []byte{0xFF, 0xD8, 0xFF, 0xE0}
	if err := u.PutJPEG(context.Background(), "observations/dev-1/2026-09-08/01J0OBS.jpg", body); err != nil {
		t.Fatalf("PutJPEG: %v", err)
	}

	if len(putter.calls) != 1 {
		t.Fatalf("PutObject called %d times, want 1", len(putter.calls))
	}
	call := putter.calls[0]
	if aws.ToString(call.Bucket) != "ew-images-dev" {
		t.Errorf("Bucket = %q, want %q", aws.ToString(call.Bucket), "ew-images-dev")
	}
	if want := "observations/dev-1/2026-09-08/01J0OBS.jpg"; aws.ToString(call.Key) != want {
		t.Errorf("Key = %q, want %q", aws.ToString(call.Key), want)
	}
	if got := aws.ToString(call.ContentType); got != "image/jpeg" {
		t.Errorf("ContentType = %q, want %q", got, "image/jpeg")
	}
	if call.Body == nil {
		t.Fatal("Body is nil")
	}
	got := make([]byte, len(body))
	if _, err := call.Body.Read(got); err != nil {
		t.Fatalf("reading Body: %v", err)
	}
	if string(got) != string(body) {
		t.Errorf("Body = %v, want %v", got, body)
	}
}
