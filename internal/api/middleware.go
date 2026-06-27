package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/pranav/samadhan/internal/domain"
)

// writeJSON serialises v as an indented JSON response with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

// errorBody is the shape every error response takes.
type errorBody struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

// writeError maps a domain/sentinel error to an HTTP status and emits a JSON
// body. Keeping this mapping in one place means handlers can simply return the
// error they get from the service and trust the transport layer to translate.
func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	code := "internal_error"

	switch {
	case errors.Is(err, domain.ErrNotFound):
		status, code = http.StatusNotFound, "not_found"
	case errors.Is(err, domain.ErrInvalidInput):
		status, code = http.StatusBadRequest, "invalid_input"
	case errors.Is(err, domain.ErrNotAnalyzed):
		status, code = http.StatusConflict, "not_analyzed"
	case errors.Is(err, domain.ErrAlreadySettled):
		status, code = http.StatusConflict, "already_settled"
	case errors.Is(err, domain.ErrNotNegotiating):
		status, code = http.StatusConflict, "not_negotiating"
	}

	writeJSON(w, status, errorBody{Error: err.Error(), Code: code})
}

// decodeJSON reads a JSON request body into dst, rejecting unknown fields and
// oversized payloads. It returns an ErrInvalidInput-wrapped error on failure so
// the standard error mapper produces a 400.
func decodeJSON(r *http.Request, dst any) error {
	const maxBody = 1 << 20 // 1 MiB is ample for a dispute intake form
	r.Body = http.MaxBytesReader(nil, r.Body, maxBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return domain.WrapInvalid(err)
	}
	return nil
}

// --- cross-cutting HTTP middleware ----------------------------------------

// statusRecorder captures the response code so the logging middleware can
// report it.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// chain applies middlewares in order, outermost first.
func chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// withLogging records method, path, status and latency for every request.
func withLogging(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			log.Info("http",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"dur", time.Since(start).String(),
			)
		})
	}
}

// withRecover converts a panic into a 500 rather than killing the connection.
func withRecover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("panic recovered", "err", rec, "path", r.URL.Path)
					writeJSON(w, http.StatusInternalServerError, errorBody{
						Error: "internal server error",
						Code:  "panic",
					})
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// withCORS allows the bundled single-page UI (and local tools like curl from a
// browser console) to call the API during a demo.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
