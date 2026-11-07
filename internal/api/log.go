package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

type ctxKey int

const logKey ctxKey = 0

// requestFields collects per-request details for the single structured log
// line emitted by withLogging.
type requestFields struct {
	InvoiceID string
	Engine    string
	Code      string
	Error     string
	WaitMS    int64
	RenderMS  int64
	UploadMS  int64
	Bytes     int
}

func fieldsFrom(ctx context.Context) *requestFields {
	f, _ := ctx.Value(logKey).(*requestFields)
	return f
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// withLogging emits exactly one JSON log line per request and records the
// request counter. Payload contents are never logged.
func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		fields := &requestFields{Code: "ok"}
		r = r.WithContext(context.WithValue(r.Context(), logKey, fields))

		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)

		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = ulid.Make().String()
		}

		attrs := []any{
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
			"http_status", sw.status,
			"code", fields.Code,
			"total_ms", time.Since(start).Milliseconds(),
			"wait_ms", fields.WaitMS,
			"render_ms", fields.RenderMS,
			"upload_ms", fields.UploadMS,
			"bytes", fields.Bytes,
			"remote_ip", r.RemoteAddr,
		}
		if fields.InvoiceID != "" {
			attrs = append(attrs, "invoice_id", fields.InvoiceID)
		}
		if fields.Engine != "" {
			attrs = append(attrs, "engine", fields.Engine)
		}
		if fields.Error != "" {
			attrs = append(attrs, "error", fields.Error)
		}

		if sw.status >= 500 {
			s.log.Error("request", attrs...)
		} else {
			s.log.Info("request", attrs...)
		}

		requestsTotal.WithLabelValues(endpointLabel(r.URL.Path), strconv.Itoa(sw.status)).Inc()
	})
}

func endpointLabel(path string) string {
	if strings.HasPrefix(path, "/invoices/") {
		return "/invoices/{id}"
	}
	return path
}
