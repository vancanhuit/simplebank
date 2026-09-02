package mail

import (
	"fmt"
	"strings"
	"testing"

	"github.com/vancanhuit/simplebank/internal/config"
)

func TestNewSMTPMailerPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     config.Config
		wantTLS string
		wantErr string
	}{
		{name: "mandatory TLS", cfg: config.Config{SMTPHost: "smtp.example.com", SMTPPort: 587}, wantTLS: "TLSMandatory"},
		{name: "implicit TLS", cfg: config.Config{SMTPHost: "smtp.example.com", SMTPPort: 465, SMTPSSL: true}, wantTLS: "TLSMandatory"},
		{name: "explicit insecure", cfg: config.Config{SMTPHost: "localhost", SMTPPort: 1025, SMTPInsecure: true}, wantTLS: "NoTLS"},
		{name: "reject plaintext auth", cfg: config.Config{SMTPHost: "localhost", SMTPPort: 1025, SMTPInsecure: true, SMTPUsername: "user"}, wantErr: "smtp auth requires TLS"},
		{name: "missing CA", cfg: config.Config{SMTPHost: "smtp.example.com", SMTPPort: 587, SMTPTLSCAFile: "missing.pem"}, wantErr: "read smtp tls ca file"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			mailer, err := NewSMTPMailer(test.cfg)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := mailer.client.TLSPolicy(); got != test.wantTLS {
				t.Errorf("TLS policy = %q, want %q", got, test.wantTLS)
			}
			if got, want := mailer.client.ServerAddr(), fmt.Sprintf("%s:%d", test.cfg.SMTPHost, test.cfg.SMTPPort); got != want {
				t.Errorf("server address = %q, want %q", got, want)
			}
		})
	}
}
