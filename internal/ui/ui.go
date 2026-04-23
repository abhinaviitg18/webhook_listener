package ui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// UI is the embedded frontend dist directory.
//
//go:embed all:dist
var UI embed.FS

// StaticHandler serves the embedded frontend.
func StaticHandler() http.Handler {
	dist, err := fs.Sub(UI, "dist")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(dist))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		f, err := dist.Open(path)
		if err != nil {
			// File not found, serve index.html for SPA routing
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}
		f.Close()

		fileServer.ServeHTTP(w, r)
	})
}
