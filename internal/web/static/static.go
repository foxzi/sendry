package static

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed css/*
var staticFS embed.FS

// Handler returns an http.Handler that serves static files
func Handler() http.Handler {
	fsys, _ := fs.Sub(staticFS, ".")
	return http.FileServer(http.FS(fsys))
}
