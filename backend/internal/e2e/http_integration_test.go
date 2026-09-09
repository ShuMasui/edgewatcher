//go:build integration

package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ShuMasui/edgewatcher/backend/internal/ids"
)

// These tests drive the whole system over real HTTP, through the API
// Gateway model in gateway_integration_test.go. What they add over the
// direct-call tests is everything between a handler's return value and what
// a client actually receives: the status code, the serialized body, the
// route the request was dispatched on, and the authorizer's verdict.
//
// The status codes are the point. docs/05-backend.md §1.6 fixes their
// meanings because the web and the device branch on them — 409 means "stop
// and re-issue", 403 on a device route means "wipe credentials and show the
// QR again", 429 means "you are at the limit". Until now that table was
// enforced only by internal/apierr's unit test over a Go map.

type apiClient struct {
	t    *testing.T
	base string
	hc   *http.Client
}

func newAPIClient(t *testing.T, baseURL string) *apiClient {
	return &apiClient{t: t, base: baseURL, hc: &http.Client{Timeout: 30 * time.Second}}
}

// call issues a request and returns the status and raw body. The body is
// returned as bytes, not decoded, so assertions can look at the wire form.
func (c *apiClient) call(method, path, auth, contentType string, body []byte) (int, []byte) {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		c.t.Fatalf("build request: %v", err)
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		c.t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, raw
}

func (c *apiClient) json(method, path, auth, body string) (int, []byte) {
	c.t.Helper()
	var payload []byte
	if body != "" {
		payload = []byte(body)
	}
	return c.call(method, path, auth, "application/json", payload)
}

// errorBody is the envelope docs/05-backend.md §1.6 fixes for every route.
type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func assertStatus(t *testing.T, got, want int, raw []byte, what string) {
	t.Helper()
	if got != want {
		t.Fatalf("%s: status = %d, want %d (body: %s)", what, got, want, raw)
	}
}

func assertErrorCode(t *testing.T, raw []byte, wantCode string) {
	t.Helper()
	var e errorBody
	if err := json.Unmarshal(raw, &e); err != nil {
		t.Fatalf("error body is not the documented envelope: %s", raw)
	}
	if e.Error.Code != wantCode {
		t.Fatalf("error code = %q, want %q (body: %s)", e.Error.Code, wantCode, raw)
	}
	if e.Error.Message == "" {
		t.Errorf("error body has no message, which the web renders: %s", raw)
	}
}

func decodeJSON[T any](t *testing.T, raw []byte) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return out
}

// multipartUpload builds the body a device actually sends.
func multipartUpload(t *testing.T, capturedAt time.Time) (contentType string, body []byte, observationID string) {
	t.Helper()
	observationID, err := ids.NewULID(capturedAt)
	if err != nil {
		t.Fatalf("mint observation id: %v", err)
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	parts := map[string][]byte{
		"image":     []byte("full-image-bytes"),
		"thumbnail": []byte("thumb-bytes"),
		"metadata": []byte(`{"observationId":"` + observationID +
			`","capturedAt":"` + capturedAt.UTC().Format(time.RFC3339) + `","lat":35.68,"lng":139.76}`),
	}
	for _, name := range []string{"image", "thumbnail", "metadata"} {
		fw, err := w.CreateFormFile(name, name)
		if err != nil {
			t.Fatalf("CreateFormFile(%q): %v", name, err)
		}
		if _, err := fw.Write(parts[name]); err != nil {
			t.Fatalf("write %q: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return w.FormDataContentType(), buf.Bytes(), observationID
}

// --- the walk ------------------------------------------------------------

// TestHTTP_QRPath walks registration through upload over HTTP, asserting the
// status code at every step. Every request here is dispatched by path — the
// route key is resolved by the gateway model, not supplied by the test — so
// a handler registered under a wrong key fails here rather than at deploy.
func TestHTTP_QRPath(t *testing.T) {
	st := newStack(t)
	server := newGatewayServer(t, st)
	c := newAPIClient(t, server.URL)
	const owner = "Bearer owner-http"

	// 1. app-config
	status, raw := c.json(http.MethodGet, "/app-config", owner, "")
	assertStatus(t, status, http.StatusOK, raw, "GET /app-config")
	cfg := decodeJSON[map[string]any](t, raw)
	if cfg["deviceLimit"] != float64(10) {
		t.Errorf("deviceLimit = %v, want 10", cfg["deviceLimit"])
	}

	// 2. create a device
	status, raw = c.json(http.MethodPost, "/devices", owner, `{"name":"玄関"}`)
	assertStatus(t, status, http.StatusOK, raw, "POST /devices")
	created := decodeJSON[struct {
		Device struct {
			DeviceID string `json:"deviceId"`
			Status   string `json:"status"`
		} `json:"device"`
		PairingSession struct {
			PairingCode string `json:"pairingCode"`
		} `json:"pairingSession"`
	}](t, raw)
	deviceID := created.Device.DeviceID
	code := created.PairingSession.PairingCode
	if deviceID == "" || code == "" {
		t.Fatalf("POST /devices body: %s", raw)
	}

	// 3. the list shows it, with the QR attached
	status, raw = c.json(http.MethodGet, "/devices", owner, "")
	assertStatus(t, status, http.StatusOK, raw, "GET /devices")
	if !bytes.Contains(raw, []byte(`"activePairingSession":{`)) {
		t.Errorf("a PENDING device should carry its session on the wire: %s", raw)
	}

	// 4. the device scans
	status, raw = c.json(http.MethodPost, "/device/pair", "",
		`{"pairingCode":"`+code+`","deviceInfo":{"model":"Pixel 8"}}`)
	assertStatus(t, status, http.StatusOK, raw, "POST /device/pair")
	paired := decodeJSON[struct {
		DeviceID     string `json:"deviceId"`
		DeviceSecret string `json:"deviceSecret"`
	}](t, raw)
	if paired.DeviceID != deviceID {
		t.Fatalf("pair resolved to %q, want %q", paired.DeviceID, deviceID)
	}

	// 5. the same QR again: 409, and the code says WHICH 409.
	//    The device distinguishes consumed from expired (04-native §1.4),
	//    and the status alone cannot carry that.
	status, raw = c.json(http.MethodPost, "/device/pair", "",
		`{"pairingCode":"`+code+`","deviceInfo":{}}`)
	assertStatus(t, status, http.StatusConflict, raw, "replayed POST /device/pair")
	assertErrorCode(t, raw, "PAIRING_CODE_CONSUMED")

	// 6. exchange for a session
	status, raw = c.json(http.MethodPost, "/device/token", "",
		`{"deviceId":"`+deviceID+`","deviceSecret":"`+paired.DeviceSecret+`"}`)
	assertStatus(t, status, http.StatusOK, raw, "POST /device/token")
	sessionToken := decodeJSON[struct {
		SessionToken string `json:"sessionToken"`
	}](t, raw).SessionToken

	// 7. upload, through the real Lambda authorizer
	ct, body, observationID := multipartUpload(t, st.clock.Now().Add(-time.Minute))
	status, raw = c.call(http.MethodPost, "/device/uploads", sessionToken, ct, body)
	assertStatus(t, status, http.StatusOK, raw, "POST /device/uploads")
	if next := decodeJSON[map[string]map[string]any](t, raw); next["nextConfig"]["intervalMinutes"] != float64(5) {
		t.Errorf("nextConfig = %v, want intervalMinutes 5", next)
	}

	// 8. the photo is in the day's history
	status, raw = c.json(http.MethodGet, "/devices/"+deviceID+"/observations?date=2026-09-08", owner, "")
	assertStatus(t, status, http.StatusOK, raw, "GET observations")
	obs := decodeJSON[[]map[string]any](t, raw)
	if len(obs) != 1 || obs[0]["observationId"] != observationID {
		t.Fatalf("observations = %s", raw)
	}
	if _, present := obs[0]["imageUrl"]; present {
		t.Errorf("a list response must omit imageUrl (05-backend §1.5): %s", raw)
	}

	// 9. the full image, on demand
	status, raw = c.json(http.MethodGet,
		"/observations/"+observationID+"/image?deviceId="+deviceID, owner, "")
	assertStatus(t, status, http.StatusOK, raw, "GET observation image")
	if img := decodeJSON[map[string]any](t, raw); img["imageUrl"] == "" || img["expiresAt"] == nil {
		t.Errorf("image response = %s", raw)
	}

	// 10. logout is 204 with an empty body
	status, raw = c.json(http.MethodPost, "/device/logout", sessionToken, "")
	assertStatus(t, status, http.StatusNoContent, raw, "POST /device/logout")
	if len(raw) != 0 {
		t.Errorf("204 must carry no body, got %q", raw)
	}
}

// TestHTTP_ErrorContract pins the status codes docs/05-backend.md §1.6
// assigns meanings to. Each one drives a different client behaviour, so a
// wrong status is a wrong behaviour, not a cosmetic difference.
func TestHTTP_ErrorContract(t *testing.T) {
	st := newStack(t)
	server := newGatewayServer(t, st)
	c := newAPIClient(t, server.URL)
	const owner = "Bearer owner-errors"
	const stranger = "Bearer someone-else"

	status, raw := c.json(http.MethodPost, "/devices", owner, `{"name":"玄関"}`)
	assertStatus(t, status, http.StatusOK, raw, "setup")
	deviceID := decodeJSON[struct {
		Device struct {
			DeviceID string `json:"deviceId"`
		} `json:"device"`
	}](t, raw).Device.DeviceID

	t.Run("400 malformed input is not retryable", func(t *testing.T) {
		status, raw := c.json(http.MethodPost, "/devices", owner, `{"name":""}`)
		assertStatus(t, status, http.StatusBadRequest, raw, "empty name")
		assertErrorCode(t, raw, "VALIDATION_ERROR")

		status, raw = c.json(http.MethodPatch, "/devices/"+deviceID, owner, `{"interval":7}`)
		assertStatus(t, status, http.StatusBadRequest, raw, "interval outside the offered set")
		assertErrorCode(t, raw, "VALIDATION_ERROR")

		// ?deviceId= is required (G11): without it there is nothing to run
		// the ownership check against.
		status, raw = c.json(http.MethodGet, "/observations/obs-1/image", owner, "")
		assertStatus(t, status, http.StatusBadRequest, raw, "image without deviceId")
		assertErrorCode(t, raw, "VALIDATION_ERROR")
	})

	t.Run("403 someone else's device", func(t *testing.T) {
		status, raw := c.json(http.MethodGet, "/devices/"+deviceID+"/observations?date=2026-09-08", stranger, "")
		assertStatus(t, status, http.StatusForbidden, raw, "stranger reading observations")
		assertErrorCode(t, raw, "FORBIDDEN")
	})

	t.Run("404 nonexistent device", func(t *testing.T) {
		status, raw := c.json(http.MethodPatch, "/devices/01JDOESNOTEXIST00000000", owner, `{"name":"x"}`)
		assertStatus(t, status, http.StatusNotFound, raw, "patching a missing device")
		assertErrorCode(t, raw, "DEVICE_NOT_FOUND")
	})

	t.Run("404 unknown pairing code is the only one the device retries", func(t *testing.T) {
		status, raw := c.json(http.MethodPost, "/device/pair", "",
			`{"pairingCode":"NOSUCHCODE","deviceInfo":{}}`)
		assertStatus(t, status, http.StatusNotFound, raw, "unknown pairing code")
		assertErrorCode(t, raw, "PAIRING_NOT_FOUND")
	})

	t.Run("401 no token at all on a web route", func(t *testing.T) {
		status, raw := c.json(http.MethodGet, "/devices", "", "")
		assertStatus(t, status, http.StatusUnauthorized, raw, "unauthenticated")
	})

	t.Run("204 delete, then the device is gone", func(t *testing.T) {
		status, raw := c.json(http.MethodDelete, "/devices/"+deviceID, owner, "")
		assertStatus(t, status, http.StatusNoContent, raw, "DELETE")

		status, raw = c.json(http.MethodGet, "/devices", owner, "")
		assertStatus(t, status, http.StatusOK, raw, "list after delete")
		if strings.TrimSpace(string(raw)) != "[]" {
			t.Errorf("list after delete = %s, want []", raw)
		}
	})

	t.Run("429 at the device limit", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			status, raw := c.json(http.MethodPost, "/devices", owner, `{"name":"埋める"}`)
			assertStatus(t, status, http.StatusOK, raw, "filling the quota")
		}
		status, raw := c.json(http.MethodPost, "/devices", owner, `{"name":"11台目"}`)
		assertStatus(t, status, http.StatusTooManyRequests, raw, "over the limit")
		assertErrorCode(t, raw, "DEVICE_LIMIT_EXCEEDED")
	})
}

// TestHTTP_DeviceRouteDenialIs403 is the status the device's whole
// self-repair loop hangs on. A denied Lambda authorizer produces 403, not
// 401 — docs/06-auth.md §5 was written around 401 and docs/04-native.md
// §1.4 still says 401, which is the discrepancy Task 14 has to correct.
// Pinning it here means the code and the docs can be compared against
// something that actually runs.
func TestHTTP_DeviceRouteDenialIs403(t *testing.T) {
	st := newStack(t)
	server := newGatewayServer(t, st)
	c := newAPIClient(t, server.URL)

	ct, body, _ := multipartUpload(t, st.clock.Now().Add(-time.Minute))

	// A token that was presented and rejected: the authorizer ran and said
	// no, which API Gateway reports as 403.
	for name, token := range map[string]string{
		"garbage token":  "01JNOSUCHDEVICE0000000.deadbeef",
		"malformed":      "not-a-token",
		"wrong scheme":   "Basic 01JNOSUCHDEVICE0000000.deadbeef",
		"empty deviceId": ".deadbeef",
	} {
		t.Run(name, func(t *testing.T) {
			status, raw := c.call(http.MethodPost, "/device/uploads", token, ct, body)
			assertStatus(t, status, http.StatusForbidden, raw, "denied upload")
		})
	}

	// No token at all is a different case, and the difference is not
	// cosmetic: identity_sources = ["$request.header.Authorization"] makes
	// API Gateway answer 401 without ever invoking the authorizer. This is
	// the state a device is in immediately after wiping its credentials, so
	// a device that treats 401 and 403 alike would be relying on a status
	// the gateway never sends for the case it thinks it is handling.
	t.Run("no token at all is 401, not 403", func(t *testing.T) {
		status, raw := c.call(http.MethodPost, "/device/uploads", "", ct, body)
		assertStatus(t, status, http.StatusUnauthorized, raw, "upload without a token")
	})

	// And nothing was written on the way through.
	if len(st.uploader.objs) != 0 {
		t.Errorf("a denied request still wrote to S3: %v", st.uploader.objs)
	}
}

// TestRouteTableMatchesTerraform keeps the gateway model honest.
//
// The model above is only evidence about production if its route table is
// production's route table. infra/envs/dev/main.tf is where the real one
// lives, so this reads it rather than trusting the copy — otherwise adding
// a route to Terraform and forgetting it here would leave the new route
// untested while every existing test stayed green.
func TestRouteTableMatchesTerraform(t *testing.T) {
	src, err := os.ReadFile("../../../infra/envs/dev/main.tf")
	if err != nil {
		t.Fatalf("read the Terraform route table: %v", err)
	}

	for _, tc := range []struct {
		terraformVar string
		modelled     []string
	}{
		{"web_routes", webRouteKeys},
		{"device_public_routes", devicePublicRouteKeys},
		{"device_routes", deviceRouteKeys},
	} {
		t.Run(tc.terraformVar, func(t *testing.T) {
			declared := terraformStringList(t, string(src), tc.terraformVar)
			got := append([]string(nil), tc.modelled...)
			sort.Strings(declared)
			sort.Strings(got)
			if !reflect.DeepEqual(declared, got) {
				t.Errorf("route table drift.\n  terraform: %v\n  modelled:  %v", declared, got)
			}
		})
	}
}

func terraformStringList(t *testing.T, src, name string) []string {
	t.Helper()
	block := regexp.MustCompile(`(?s)` + regexp.QuoteMeta(name) + `\s*=\s*\[(.*?)\]`).FindStringSubmatch(src)
	if block == nil {
		t.Fatalf("%s not found in the Terraform config", name)
	}
	var out []string
	for _, m := range regexp.MustCompile(`"([^"]+)"`).FindAllStringSubmatch(block[1], -1) {
		out = append(out, m[1])
	}
	return out
}
