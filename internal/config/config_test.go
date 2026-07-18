package config

import "testing"

func TestValidate(t *testing.T) {
	t.Parallel()
	valid := Config{
		DBSource:  "postgres://u:p@localhost:5432/db",
		JWTSecret: "01234567890123456789012345678901",
		SMTPFrom:  "no-reply@example.com",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}

	tests := []struct {
		name string
		cfg  Config
	}{
		{"missing db", Config{JWTSecret: "01234567890123456789012345678901", SMTPFrom: "a@b.c"}},
		{"short secret", Config{DBSource: "x", JWTSecret: "short", SMTPFrom: "a@b.c"}},
		{"31-char secret", Config{DBSource: "x", JWTSecret: "0123456789012345678901234567890", SMTPFrom: "a@b.c"}},
		{"missing from", Config{DBSource: "x", JWTSecret: "01234567890123456789012345678901"}},
		{"tls cert without key", Config{DBSource: "x", JWTSecret: "01234567890123456789012345678901", SMTPFrom: "a@b.c", TLSCertFile: "cert.pem"}},
		{"tls key without cert", Config{DBSource: "x", JWTSecret: "01234567890123456789012345678901", SMTPFrom: "a@b.c", TLSKeyFile: "key.pem"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := tt.cfg.Validate(); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}
