// Package web serves the built frontend from inside the binary.
//
// The dist directory is copied here by the build (see the Makefile / README),
// so a release is one file: server, database driver and app together.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var dist embed.FS

// Handler serves the built single-page app, or nil if this binary was built
// without one — which is the normal case in development, where Vite serves
// the frontend and proxies /api here.
func Handler() http.Handler {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil
	}
	return &spa{fs: sub, files: http.FileServer(http.FS(sub))}
}

type spa struct {
	fs    fs.FS
	files http.Handler
}

// ServeHTTP falls back to index.html for anything that is not a real file, so
// a deep link like /t/9f3a… is handled by the client router rather than 404ing.
func (s *spa) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")

	if name != "" {
		if f, err := s.fs.Open(name); err == nil {
			f.Close()
			// Hashed asset filenames are safe to cache hard; index.html is not.
			if strings.HasPrefix(name, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			s.files.ServeHTTP(w, r)
			return
		}
	}

	w.Header().Set("Cache-Control", "no-cache")
	r2 := r.Clone(r.Context())
	r2.URL.Path = "/"
	s.files.ServeHTTP(w, r2)
}
