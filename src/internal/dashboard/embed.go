// Package dashboard embeds the React dashboard static files
package dashboard

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed dist/*
var distFS embed.FS

// Handler returns an http.Handler that serves the embedded dashboard
func Handler() http.Handler {
	// Strip the "dist" prefix from the embedded filesystem
	subFS, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic("failed to create sub filesystem: " + err.Error())
	}

	fileServer := http.FileServer(http.FS(subFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Serve static files directly
		if path != "/" && hasFileExtension(path) {
			// Vite emits content-hashed filenames under /assets/, so they are
			// safe to cache forever. Everything else (favicon, etc.) may change
			// in place, so revalidate.
			if strings.HasPrefix(path, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			fileServer.ServeHTTP(w, r)
			return
		}

		// For SPA routing: serve index.html for all other paths. Never cache the
		// HTML entry point — it references the current hashed bundle, so caching
		// it would serve a stale dashboard after a deploy.
		w.Header().Set("Cache-Control", "no-cache")
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}

// hasFileExtension checks if a path has a file extension
func hasFileExtension(path string) bool {
	// Check for common static file extensions
	extensions := []string{
		".html", ".css", ".js", ".json",
		".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".webp",
		".woff", ".woff2", ".ttf", ".eot",
		".mp3", ".wav", ".ogg",
		".mp4", ".webm",
		".map", ".txt", ".xml",
	}

	for _, ext := range extensions {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}
