package storage

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestKey(t *testing.T) {
	if got, want := Key("01J8ZZZZZZZZZZZZZZZZZZZZZZ"), "invoices/01J8ZZZZZZZZZZZZZZZZZZZZZZ.pdf"; got != want {
		t.Fatalf("Key = %q, want %q", got, want)
	}
}

func testClient(t *testing.T, endpoint string) *Client {
	t.Helper()
	c, err := New(Config{
		Endpoint:     endpoint,
		Region:       "us-east-1",
		Bucket:       "invoices",
		AccessKey:    "test",
		SecretKey:    "test",
		UsePathStyle: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestUploadRetries(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := testClient(t, srv.URL)
	if err := c.Upload(context.Background(), "invoices/x.pdf", []byte("pdf"), "application/pdf"); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestUploadGivesUp(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := testClient(t, srv.URL)
	if err := c.Upload(context.Background(), "invoices/x.pdf", []byte("pdf"), "application/pdf"); err == nil {
		t.Fatal("want error after retries are exhausted")
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestDownloadNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `<?xml version="1.0"?><Error><Code>NoSuchKey</Code><Message>missing</Message></Error>`)
	}))
	defer srv.Close()

	c := testClient(t, srv.URL)
	if _, _, err := c.Download(context.Background(), "invoices/x.pdf"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestEnsureBucketCreatesMissing(t *testing.T) {
	var head, put bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			head = true
			w.WriteHeader(http.StatusNotFound)
		case http.MethodPut:
			put = true
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	c := testClient(t, srv.URL)
	if err := c.EnsureBucket(context.Background()); err != nil {
		t.Fatalf("EnsureBucket: %v", err)
	}
	if !head || !put {
		t.Fatalf("head=%v put=%v, want both", head, put)
	}
}

func TestEnsureBucketExisting(t *testing.T) {
	var put bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			put = true
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := testClient(t, srv.URL)
	if err := c.EnsureBucket(context.Background()); err != nil {
		t.Fatalf("EnsureBucket: %v", err)
	}
	if put {
		t.Fatal("bucket was recreated although it exists")
	}
}
