package api

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/labstack/echo/v5"
)

// RegisterSPA serves the embedded single-page application as a catch-all route.
// It must be called after the API and health routes are registered so those
// take precedence; the wildcard only handles paths the router did not match.
// Real asset requests are served from the embedded filesystem, while any unknown
// path resolves to index.html so client-side routes and refreshes load the app
// shell. A nil filesystem disables SPA serving (used by unit tests).
func (s *Server) RegisterSPA(dist fs.FS) {
	if dist == nil {
		return
	}
	fileServer := http.FileServerFS(dist)
	s.router.GET("/*", func(c *echo.Context) error {
		return serveSPA(c, dist, fileServer)
	})
}

func serveSPA(c *echo.Context, dist fs.FS, fileServer http.Handler) error {
	r := c.Request()
	// Normalize the URL path to an embedded-filesystem path (no leading slash).
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" {
		name = "index.html"
	}

	// Unmatched API paths must not fall through to the SPA; return a JSON 404 so
	// API clients always receive a consistent error shape rather than HTML.
	if strings.HasPrefix(name, "api/") {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}

	info, err := fs.Stat(dist, name)
	if err != nil || info.IsDir() {
		// Unknown path or directory: serve the app shell for client-side routing.
		return serveIndex(c, dist)
	}

	setCacheHeaders(c, name)
	fileServer.ServeHTTP(c.Response(), r)
	return nil
}

func serveIndex(c *echo.Context, dist fs.FS) error {
	f, err := dist.Open("index.html")
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	c.Response().Header().Set("Cache-Control", "no-cache")
	return c.Blob(http.StatusOK, "text/html; charset=utf-8", data)
}

// setCacheHeaders marks Vite's content-hashed assets as immutable and long-lived
// while keeping other files (like index.html) revalidated on every load.
func setCacheHeaders(c *echo.Context, name string) {
	if strings.HasPrefix(name, "assets/") {
		c.Response().Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		c.Response().Header().Set("Cache-Control", "no-cache")
	}
}
