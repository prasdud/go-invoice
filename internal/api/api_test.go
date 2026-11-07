package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prasdud/go-invoice/internal/domain"
	"github.com/prasdud/go-invoice/internal/renderer"
	"github.com/prasdud/go-invoice/internal/storage"
)

const validID = "01J8ZZZZZZZZZZZZZZZZZZZZZZ"

type memStore struct {
	objects    map[string][]byte
	failUpload bool
}

func newMemStore() *memStore { return &memStore{objects: map[string][]byte{}} }

func (m *memStore) Upload(_ context.Context, key string, data []byte, _ string) error {
	if m.failUpload {
		return errors.New("upload boom")
	}
	m.objects[key] = data
	return nil
}

func (m *memStore) Download(_ context.Context, key string) (io.ReadCloser, int64, error) {
	b, ok := m.objects[key]
	if !ok {
		return nil, 0, storage.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(b)), int64(len(b)), nil
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func okRender(context.Context, domain.Engine, renderer.Options, json.RawMessage) ([]byte, error) {
	return []byte("%PDF-fake"), nil
}

func testConfig(maxRenders int) Config {
	return Config{
		BaseURL:              "http://invoice.test",
		APIKey:               "secret",
		CompanyName:          "Test Co",
		MaxConcurrentRenders: maxRenders,
		AdmissionWait:        100 * time.Millisecond,
		RenderTimeout:        time.Second,
	}
}

func post(s *Server, key, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/invoices", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("X-API-Key", key)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	return w
}

func wantError(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status = %d, want %d (body %s)", w.Code, status, w.Body.String())
	}
	var e domain.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
		t.Fatalf("error body is not JSON: %v", err)
	}
	if e.Code != code {
		t.Fatalf("code = %q, want %q", e.Code, code)
	}
}

func TestCreate(t *testing.T) {
	store := newMemStore()
	s := New(testConfig(4), okRender, store, discardLogger())

	w := post(s, "secret", `{"payload":{"invoice_number":"INV-1"}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}

	var resp domain.CreateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	if len(resp.InvoiceID) != 26 {
		t.Fatalf("generated id %q is not a ULID", resp.InvoiceID)
	}
	if resp.URL != "http://invoice.test/invoices/"+resp.InvoiceID {
		t.Fatalf("url = %q", resp.URL)
	}
	if got := store.objects[storage.Key(resp.InvoiceID)]; string(got) != "%PDF-fake" {
		t.Fatalf("object not stored, got %q", got)
	}
}

func TestCreateWithCallerID(t *testing.T) {
	store := newMemStore()
	s := New(testConfig(4), okRender, store, discardLogger())

	w := post(s, "secret", `{"id":"`+validID+`","payload":{}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	var resp domain.CreateResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.InvoiceID != validID {
		t.Fatalf("invoice_id = %q, want %q", resp.InvoiceID, validID)
	}
	if _, ok := store.objects[storage.Key(validID)]; !ok {
		t.Fatal("object not stored under caller id")
	}
}

func TestCreateUnauthorized(t *testing.T) {
	s := New(testConfig(4), okRender, newMemStore(), discardLogger())
	wantError(t, post(s, "", `{"payload":{}}`), http.StatusUnauthorized, "unauthorized")
	wantError(t, post(s, "wrong", `{"payload":{}}`), http.StatusUnauthorized, "unauthorized")
}

func TestCreateRequestErrors(t *testing.T) {
	s := New(testConfig(4), okRender, newMemStore(), discardLogger())

	tests := []struct {
		name   string
		body   string
		status int
		code   string
	}{
		{"malformed json", `{`, http.StatusBadRequest, "invalid_json"},
		{"unknown engine", `{"engine":"latex","payload":{}}`, http.StatusBadRequest, "unknown_engine"},
		{"short id", `{"id":"ord_12345","payload":{}}`, http.StatusBadRequest, "invalid_id"},
		{"missing payload", `{}`, http.StatusUnprocessableEntity, "invalid_payload"},
		{"bad items", `{"payload":{"items":"nope"}}`, http.StatusUnprocessableEntity, "invalid_payload"},
		{"bad item field", `{"payload":{"items":[{"quantity":"ten"}]}}`, http.StatusUnprocessableEntity, "invalid_payload"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wantError(t, post(s, "secret", tc.body), tc.status, tc.code)
		})
	}
}

func TestCreateWrongContentType(t *testing.T) {
	s := New(testConfig(4), okRender, newMemStore(), discardLogger())
	req := httptest.NewRequest(http.MethodPost, "/invoices", strings.NewReader(`{}`))
	req.Header.Set("X-API-Key", "secret")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	wantError(t, w, http.StatusUnsupportedMediaType, "unsupported_media_type")
}

func TestCreateOversizedBody(t *testing.T) {
	s := New(testConfig(4), okRender, newMemStore(), discardLogger())
	big := `{"payload":{"notes":"` + strings.Repeat("a", 1<<20) + `"}}`
	wantError(t, post(s, "secret", big), http.StatusRequestEntityTooLarge, "payload_too_large")
}

func TestCreateCJKGets422(t *testing.T) {
	render := func(context.Context, domain.Engine, renderer.Options, json.RawMessage) ([]byte, error) {
		return nil, renderer.ErrCJKRequiresHTML
	}
	s := New(testConfig(4), render, newMemStore(), discardLogger())
	wantError(t, post(s, "secret", `{"payload":{}}`), http.StatusUnprocessableEntity, "invalid_payload")
}

func TestCreateRenderFailure(t *testing.T) {
	render := func(context.Context, domain.Engine, renderer.Options, json.RawMessage) ([]byte, error) {
		return nil, errors.New("engine exploded")
	}
	s := New(testConfig(4), render, newMemStore(), discardLogger())
	wantError(t, post(s, "secret", `{"payload":{}}`), http.StatusInternalServerError, "render_failed")
}

func TestCreateRenderTimeout(t *testing.T) {
	render := func(ctx context.Context, _ domain.Engine, _ renderer.Options, _ json.RawMessage) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	cfg := testConfig(4)
	cfg.RenderTimeout = 20 * time.Millisecond
	s := New(cfg, render, newMemStore(), discardLogger())
	wantError(t, post(s, "secret", `{"payload":{}}`), http.StatusInternalServerError, "render_timeout")
}

func TestCreateStorageFailure(t *testing.T) {
	store := newMemStore()
	store.failUpload = true
	s := New(testConfig(4), okRender, store, discardLogger())
	wantError(t, post(s, "secret", `{"payload":{}}`), http.StatusBadGateway, "storage_unavailable")
}

func TestCreateOverloaded(t *testing.T) {
	started := make(chan struct{})
	block := make(chan struct{})
	var once sync.Once
	render := func(context.Context, domain.Engine, renderer.Options, json.RawMessage) ([]byte, error) {
		once.Do(func() { close(started) })
		<-block
		return []byte("%PDF"), nil
	}
	cfg := testConfig(1)
	cfg.AdmissionWait = 50 * time.Millisecond
	s := New(cfg, render, newMemStore(), discardLogger())

	done := make(chan struct{})
	go func() {
		post(s, "secret", `{"payload":{}}`)
		close(done)
	}()
	<-started

	w := post(s, "secret", `{"payload":{}}`)
	wantError(t, w, http.StatusServiceUnavailable, "overloaded")
	if w.Header().Get("Retry-After") != "1" {
		t.Fatalf("Retry-After = %q, want \"1\"", w.Header().Get("Retry-After"))
	}

	close(block)
	<-done
}

func TestDownload(t *testing.T) {
	store := newMemStore()
	store.objects[storage.Key(validID)] = []byte("%PDF-data")
	s := New(testConfig(4), okRender, store, discardLogger())

	req := httptest.NewRequest(http.MethodGet, "/invoices/"+validID, nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if w.Body.String() != "%PDF-data" {
		t.Fatalf("body = %q", w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Fatalf("content-type = %q", ct)
	}
	if cl := w.Header().Get("Content-Length"); cl != "9" {
		t.Fatalf("content-length = %q", cl)
	}
	if loc := w.Header().Get("Location"); loc != "" {
		t.Fatalf("Location header leaked: %q", loc)
	}
}

func TestDownloadErrors(t *testing.T) {
	s := New(testConfig(4), okRender, newMemStore(), discardLogger())

	req := httptest.NewRequest(http.MethodGet, "/invoices/"+validID, nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	wantError(t, w, http.StatusNotFound, "not_found")

	req = httptest.NewRequest(http.MethodGet, "/invoices/short", nil)
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	wantError(t, w, http.StatusBadRequest, "invalid_id")
}

func TestHealth(t *testing.T) {
	s := New(testConfig(4), okRender, newMemStore(), discardLogger())
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "ok") {
		t.Fatalf("health = %d %s", w.Code, w.Body.String())
	}
}
