// Package frontend embeds the built single-page application so the API server
// can serve it from the same binary. The dist directory is produced by the
// frontend build (bun run build); the app:build mise task runs frontend:build
// first so dist always exists before the Go build compiles this package.
package frontend

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// Dist returns the built SPA assets rooted at the dist directory, ready to be
// served over HTTP.
func Dist() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}
