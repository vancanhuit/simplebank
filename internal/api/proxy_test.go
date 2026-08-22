package api

import (
	"net/http"
	"testing"

	"github.com/labstack/echo/v5"
)

func TestClientIPExtractor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		trustedProxies []string
		remoteAddr     string
		xff            string
		want           string
	}{
		{
			name:       "default ignores xff from private peer",
			remoteAddr: "10.0.0.1:4321",
			xff:        "203.0.113.5",
			want:       "10.0.0.1",
		},
		{
			name:       "default ignores spoofed xff from public client",
			remoteAddr: "203.0.113.9:4321",
			xff:        "1.2.3.4",
			want:       "203.0.113.9",
		},
		{
			name:           "configured range trusts only listed proxy",
			trustedProxies: []string{"10.0.0.0/8"},
			remoteAddr:     "10.0.0.1:4321",
			xff:            "203.0.113.5",
			want:           "203.0.113.5",
		},
		{
			name:           "configured range ignores untrusted private proxy",
			trustedProxies: []string{"10.0.0.0/8"},
			remoteAddr:     "192.168.1.1:4321",
			xff:            "203.0.113.5",
			want:           "192.168.1.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			extract, err := clientIPExtractor(tt.trustedProxies)
			if err != nil {
				t.Fatalf("clientIPExtractor() error = %v", err)
			}
			req := newProxyRequest(tt.remoteAddr, tt.xff)
			if got := extract(req); got != tt.want {
				t.Errorf("RealIP = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClientIPExtractorInvalidCIDR(t *testing.T) {
	t.Parallel()
	if _, err := clientIPExtractor([]string{"not-a-cidr"}); err == nil {
		t.Fatal("expected error for invalid CIDR, got nil")
	}
}

func TestForwardedHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		host   string
		header string
		want   string
	}{
		{"no forwarded header falls back to host", "app:8080", "", "app:8080"},
		{"uses forwarded host", "app:8080", "bank.example.com", "bank.example.com"},
		{"takes first of multiple hops", "app:8080", "bank.example.com, internal", "bank.example.com"},
		{"trims surrounding space", "app:8080", " bank.example.com ", "bank.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			header := http.Header{}
			if tt.header != "" {
				header.Set(headerXForwardedHost, tt.header)
			}
			if got := forwardedHost(tt.host, header); got != tt.want {
				t.Errorf("forwardedHost = %q, want %q", got, tt.want)
			}
		})
	}
}

// newProxyRequest builds a request with the given RemoteAddr and optional
// X-Forwarded-For header for exercising the IP extractor.
func newProxyRequest(remoteAddr, xff string) *http.Request {
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remoteAddr
	if xff != "" {
		req.Header.Set(echo.HeaderXForwardedFor, xff)
	}
	return req
}
