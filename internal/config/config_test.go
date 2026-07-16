package config

import "testing"

func TestValidate(t *testing.T) {
	valid := Config{
		DBSource:  "postgres://u:p@localhost:5432/db",
		JWTSecret: "01234567890123456789012345678901",
		SMTPFrom:  "no-reply@example.com",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}

	tests := map[string]Config{
		"missing db":     {JWTSecret: "01234567890123456789012345678901", SMTPFrom: "a@b.c"},
		"short secret":   {DBSource: "x", JWTSecret: "short", SMTPFrom: "a@b.c"},
		"31-char secret": {DBSource: "x", JWTSecret: "0123456789012345678901234567890", SMTPFrom: "a@b.c"},
		"missing from":   {DBSource: "x", JWTSecret: "01234567890123456789012345678901"},
	}
	for name, cfg := range tests {
		t.Run(name, func(t *testing.T) {
			if err := cfg.Validate(); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}
