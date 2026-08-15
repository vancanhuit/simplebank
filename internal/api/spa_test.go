package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func spaTestFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":           {Data: []byte("<!doctype html><title>SimpleBank</title>")},
		"assets/app-abc123.js": {Data: []byte("console.log('hi')")},
	}
}

func TestRegisterSPAServesIndexAtRoot(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)
	s.RegisterSPA(spaTestFS())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("want html content type, got %q", ct)
	}
}

func TestRegisterSPAHeadReturnsMethodNotAllowed(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)
	s.RegisterSPA(spaTestFS())

	req := httptest.NewRequest(http.MethodHead, "/", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405 for HEAD, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestRegisterSPAFallsBackToIndexForClientRoute(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)
	s.RegisterSPA(spaTestFS())

	req := httptest.NewRequest(http.MethodGet, "/transfer", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 for client route, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "SimpleBank") {
		t.Fatalf("want app shell body, got %q", rec.Body.String())
	}
}

func TestRegisterSPAServesHashedAssetWithImmutableCache(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)
	s.RegisterSPA(spaTestFS())

	req := httptest.NewRequest(http.MethodGet, "/assets/app-abc123.js", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 for asset, got %d", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Fatalf("want immutable cache header, got %q", cc)
	}
}

func TestRegisterSPAReturnsJSONForUnknownAPIPath(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)
	s.RegisterSPA(spaTestFS())

	// A path under /api that matches no route reaches the SPA catch-all; the
	// guard must return a JSON 404 rather than serving the HTML app shell.
	req := httptest.NewRequest(http.MethodGet, "/api/unknown", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 for unknown API path, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("want json error, got content type %q", ct)
	}
	if strings.Contains(rec.Body.String(), "SimpleBank") {
		t.Fatalf("API path must not serve the SPA shell, got %q", rec.Body.String())
	}
}

func TestRegisterSPANilFSDisablesServing(t *testing.T) {
	t.Parallel()
	s := newTestServer(t)
	s.RegisterSPA(nil)

	// With no filesystem the catch-all is never registered, so the root path is
	// left unmatched and the SPA shell is never served.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("want SPA serving disabled, got 200")
	}
	if strings.Contains(rec.Body.String(), "SimpleBank") {
		t.Fatalf("SPA must not be served when disabled, got %q", rec.Body.String())
	}
}
