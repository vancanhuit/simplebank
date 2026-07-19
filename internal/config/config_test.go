package config

import "testing"

func TestParseTransferLimits(t *testing.T) {
	t.Parallel()

	empty, err := parseTransferLimits("")
	if err != nil {
		t.Fatalf("empty input should not error, got %v", err)
	}
	if empty != nil {
		t.Errorf("empty input should yield no limits, got %v", empty)
	}

	limits, err := parseTransferLimits(`{"USD":{"max_per_transfer":100000,"daily":500000},"VND":{"max_per_transfer":2000000000}}`)
	if err != nil {
		t.Fatalf("valid JSON should parse, got %v", err)
	}
	if got := limits["USD"]; got.MaxPerTransfer != 100000 || got.Daily != 500000 {
		t.Errorf("USD limit = %+v, want {100000 500000}", got)
	}
	// VND omits daily, so it stays disabled (zero) while max applies.
	if got := limits["VND"]; got.MaxPerTransfer != 2000000000 || got.Daily != 0 {
		t.Errorf("VND limit = %+v, want {2000000000 0}", got)
	}

	if _, err := parseTransferLimits("not json"); err == nil {
		t.Error("malformed JSON should error")
	}
}

func TestValidateRejectsBadTransferLimits(t *testing.T) {
	t.Parallel()
	_, err := parseTransferLimits("{bad")
	cfg := Config{
		DBSource:          "x",
		JWTSecret:         "01234567890123456789012345678901",
		SMTPFrom:          "a@b.c",
		transferLimitsErr: err,
	}
	if cfg.Validate() == nil {
		t.Error("expected Validate to reject a transfer-limits parse error")
	}
}

func TestLimitFor(t *testing.T) {
	t.Parallel()
	cfg := Config{TransferLimits: map[string]CurrencyLimit{"USD": {MaxPerTransfer: 100000, Daily: 500000}}}
	if got := cfg.LimitFor("USD"); got.MaxPerTransfer != 100000 {
		t.Errorf("USD MaxPerTransfer = %d, want 100000", got.MaxPerTransfer)
	}
	// An unconfigured currency resolves to a zero-value (all limits disabled).
	if got := cfg.LimitFor("EUR"); got != (CurrencyLimit{}) {
		t.Errorf("unconfigured currency should disable limits, got %+v", got)
	}
}

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
