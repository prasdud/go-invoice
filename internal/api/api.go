// Package api implements the synchronous invoice pipeline: authenticate,
// validate, render, upload, respond. This is the single place where the
// request workflow is composed.
package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/prasdud/go-invoice/internal/domain"
	"github.com/prasdud/go-invoice/internal/renderer"
	"github.com/prasdud/go-invoice/internal/storage"
	"github.com/prasdud/go-invoice/internal/validate"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const maxBodyBytes = 1 << 20

// Render renders an invoice payload to PDF bytes.
type Render func(ctx context.Context, engine domain.Engine, opts renderer.Options, payload json.RawMessage) ([]byte, error)

// Storage persists and serves PDF artifacts.
type Storage interface {
	Upload(ctx context.Context, key string, data []byte, contentType string) error
	Download(ctx context.Context, key string) (io.ReadCloser, int64, error)
}

type Config struct {
	BaseURL              string
	APIKey               string
	CompanyName          string
	CompanyRGB           [3]int
	Logo                 *renderer.Logo
	MaxConcurrentRenders int
	AdmissionWait        time.Duration
	RenderTimeout        time.Duration
}

type Server struct {
	cfg    Config
	sem    chan struct{}
	render Render
	store  Storage
	log    *slog.Logger
}

func New(cfg Config, render Render, store Storage, log *slog.Logger) *Server {
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if log == nil {
		log = slog.Default()
	}
	return &Server{
		cfg:    cfg,
		sem:    make(chan struct{}, cfg.MaxConcurrentRenders),
		render: render,
		store:  store,
		log:    log,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"status":"ok"}`)
	})
	mux.Handle("GET /metrics", promhttp.Handler())
	mux.HandleFunc("POST /invoices", s.handleCreate)
	mux.HandleFunc("GET /invoices/{id}", s.handleDownload)
	return s.withLogging(mux)
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		s.writeError(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type must be application/json")
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-API-Key")), []byte(s.cfg.APIKey)) != 1 {
		s.writeError(w, r, http.StatusUnauthorized, "unauthorized", "missing or invalid API key")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var req domain.InvoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			s.writeError(w, r, http.StatusRequestEntityTooLarge, "payload_too_large", "request body exceeds 1 MiB")
			return
		}
		s.writeError(w, r, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}
	if req.Engine == "" {
		req.Engine = domain.EngineBasic
	}
	if err := validate.InvoiceRequest(req); err != nil {
		var ve *validate.Error
		if errors.As(err, &ve) {
			status := http.StatusUnprocessableEntity
			if ve.Code == "invalid_id" || ve.Code == "unknown_engine" {
				status = http.StatusBadRequest
			}
			s.writeError(w, r, status, ve.Code, ve.Message)
			return
		}
		s.writeError(w, r, http.StatusUnprocessableEntity, "invalid_payload", err.Error())
		return
	}

	id := req.ID
	if id == "" {
		id = ulid.Make().String()
	}
	if f := fieldsFrom(r.Context()); f != nil {
		f.InvoiceID = id
		f.Engine = string(req.Engine)
	}

	waitStart := time.Now()
	release, ok := s.acquire(r.Context())
	renderWait.Observe(time.Since(waitStart).Seconds())
	if f := fieldsFrom(r.Context()); f != nil {
		f.WaitMS = time.Since(waitStart).Milliseconds()
	}
	if !ok {
		if r.Context().Err() != nil {
			return // client is gone
		}
		w.Header().Set("Retry-After", "1")
		s.writeError(w, r, http.StatusServiceUnavailable, "overloaded", "render capacity exhausted; retry later")
		return
	}
	rendersInFlight.Inc()
	defer func() {
		rendersInFlight.Dec()
		release()
	}()

	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.RenderTimeout)
	renderStart := time.Now()
	pdf, err := s.render(ctx, req.Engine, renderer.Options{
		CompanyName:  s.cfg.CompanyName,
		PrimaryColor: s.cfg.CompanyRGB,
		Logo:         s.cfg.Logo,
	}, req.Payload)
	cancel()
	renderDuration.WithLabelValues(string(req.Engine)).Observe(time.Since(renderStart).Seconds())
	if f := fieldsFrom(r.Context()); f != nil {
		f.RenderMS = time.Since(renderStart).Milliseconds()
	}
	if err != nil {
		switch {
		case errors.Is(err, renderer.ErrCJKRequiresHTML):
			s.writeError(w, r, http.StatusUnprocessableEntity, "invalid_payload", err.Error())
		case errors.Is(err, context.DeadlineExceeded):
			s.writeError(w, r, http.StatusInternalServerError, "render_timeout", "render exceeded the timeout")
		default:
			s.log.Error("render failed", "invoice_id", id, "engine", req.Engine, "error", err)
			s.writeError(w, r, http.StatusInternalServerError, "render_failed", "render failed")
		}
		return
	}

	uploadStart := time.Now()
	err = s.store.Upload(r.Context(), storage.Key(id), pdf, "application/pdf")
	uploadDuration.Observe(time.Since(uploadStart).Seconds())
	if f := fieldsFrom(r.Context()); f != nil {
		f.UploadMS = time.Since(uploadStart).Milliseconds()
		f.Bytes = len(pdf)
	}
	if err != nil {
		s.log.Error("upload failed", "invoice_id", id, "error", err)
		s.writeError(w, r, http.StatusBadGateway, "storage_unavailable", "storage upload failed")
		return
	}

	s.writeJSON(w, http.StatusOK, domain.CreateResponse{
		InvoiceID: id,
		URL:       s.cfg.BaseURL + "/invoices/" + id,
	})
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if f := fieldsFrom(r.Context()); f != nil {
		f.InvoiceID = id
	}
	if !validate.ValidID(id) {
		s.writeError(w, r, http.StatusBadRequest, "invalid_id", "id must match [A-Za-z0-9_-]{22,64}")
		return
	}

	body, size, err := s.store.Download(r.Context(), storage.Key(id))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			s.writeError(w, r, http.StatusNotFound, "not_found", "invoice not found")
			return
		}
		s.log.Error("download failed", "invoice_id", id, "error", err)
		s.writeError(w, r, http.StatusBadGateway, "storage_unavailable", "storage download failed")
		return
	}
	defer body.Close()

	w.Header().Set("Content-Type", "application/pdf")
	if size >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	if _, err := io.Copy(w, body); err != nil {
		s.log.Error("stream failed", "invoice_id", id, "error", err)
	}
}

// acquire waits up to AdmissionWait for a render slot.
func (s *Server) acquire(ctx context.Context) (func(), bool) {
	timer := time.NewTimer(s.cfg.AdmissionWait)
	defer timer.Stop()
	select {
	case s.sem <- struct{}{}:
		return func() { <-s.sem }, true
	case <-timer.C:
		return nil, false
	case <-ctx.Done():
		return nil, false
	}
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, status int, code, msg string) {
	if f := fieldsFrom(r.Context()); f != nil {
		f.Code = code
		f.Error = msg
	}
	s.writeJSON(w, status, domain.ErrorResponse{Error: msg, Code: code})
}
