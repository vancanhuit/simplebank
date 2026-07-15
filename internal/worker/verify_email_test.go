package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	store "github.com/vancanhuit/simplebank/internal/db"
	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
)

type mockStore struct {
	store.Store
	getUser           func(context.Context, string) (sqlcdb.User, error)
	createVerifyEmail func(context.Context, sqlcdb.CreateVerifyEmailParams) (sqlcdb.VerifyEmail, error)
}

func (m mockStore) GetUser(ctx context.Context, username string) (sqlcdb.User, error) {
	return m.getUser(ctx, username)
}

func (m mockStore) CreateVerifyEmail(ctx context.Context, arg sqlcdb.CreateVerifyEmailParams) (sqlcdb.VerifyEmail, error) {
	return m.createVerifyEmail(ctx, arg)
}

type mockMailer struct {
	called           bool
	to, subject, msg string
}

func (m *mockMailer) Send(_ context.Context, to, subject, htmlBody string) error {
	m.called = true
	m.to, m.subject, m.msg = to, subject, htmlBody
	return nil
}

func TestSendVerifyEmailWorker(t *testing.T) {
	st := mockStore{
		getUser: func(_ context.Context, username string) (sqlcdb.User, error) {
			return sqlcdb.User{Username: username, Email: "alice@example.com", FullName: "<b>Bob</b>"}, nil
		},
		createVerifyEmail: func(_ context.Context, arg sqlcdb.CreateVerifyEmailParams) (sqlcdb.VerifyEmail, error) {
			return sqlcdb.VerifyEmail{ID: uuid.New(), SecretCode: arg.SecretCode}, nil
		},
	}
	mailer := &mockMailer{}
	w := NewSendVerifyEmailWorker(st, mailer, "https://bank.example.com")

	job := &river.Job[SendVerifyEmailArgs]{Args: SendVerifyEmailArgs{Username: "alice"}}
	if err := w.Work(context.Background(), job); err != nil {
		t.Fatalf("Work: %v", err)
	}

	if !mailer.called {
		t.Fatal("mailer should be invoked")
	}
	if mailer.to != "alice@example.com" {
		t.Errorf("recipient = %q, want alice@example.com", mailer.to)
	}
	// The full name is attacker-controlled; it must be HTML-escaped in the body.
	if !strings.Contains(mailer.msg, "&lt;b&gt;Bob&lt;/b&gt;") {
		t.Errorf("full name not HTML-escaped in body: %q", mailer.msg)
	}
	if strings.Contains(mailer.msg, "<b>Bob</b>") {
		t.Errorf("raw HTML from full name leaked into body: %q", mailer.msg)
	}
	if !strings.Contains(mailer.msg, "/api/v1/users/verify_email?id=") {
		t.Errorf("verification link missing from body: %q", mailer.msg)
	}
}
