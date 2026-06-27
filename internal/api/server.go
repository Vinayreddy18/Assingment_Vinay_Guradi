package api

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
)

// NewServer builds the HTTP handler: the JSON API under /api/v1, a health
// probe, and the bundled single-page UI served from webDir.
func NewServer(svc *Service, webDir string, log *slog.Logger) http.Handler {
	h := NewHandlers(svc)
	mux := http.NewServeMux()

	// Go 1.22 pattern routing: method + path, with {id} wildcards.
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("POST /api/v1/disputes", h.createDispute)
	mux.HandleFunc("GET /api/v1/disputes", h.listDisputes)
	mux.HandleFunc("GET /api/v1/disputes/{id}", h.getDispute)
	mux.HandleFunc("POST /api/v1/disputes/{id}/analyze", h.analyzeDispute)
	mux.HandleFunc("POST /api/v1/disputes/{id}/offers", h.submitOffers)
	mux.HandleFunc("POST /api/v1/disputes/{id}/simulate", h.simulate)
	mux.HandleFunc("POST /api/v1/disputes/{id}/accept", h.acceptRecommended)

	// Static UI. Serve index.html at "/" and assets beneath it. If the web
	// directory is absent (e.g. a pure-API deployment) the API still works.
	if webDir != "" {
		if _, err := os.Stat(webDir); err == nil {
			fs := http.FileServer(http.Dir(webDir))
			mux.Handle("/", spaHandler(webDir, fs))
		} else {
			log.Warn("web directory not found, serving API only", "dir", webDir)
		}
	}

	return chain(mux,
		withRecover(log),
		withLogging(log),
		withCORS,
	)
}

// spaHandler serves static files, falling back to index.html for the root so
// the single-page app loads cleanly. Unknown asset paths return the file
// server's own 404.
func spaHandler(webDir string, fs http.Handler) http.Handler {
	index := filepath.Join(webDir, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.ServeFile(w, r, index)
			return
		}
		fs.ServeHTTP(w, r)
	})
}
