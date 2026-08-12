package worker

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/riverqueue/river"
)

func TestSendRegistrationNoticeArgsInsertOpts(t *testing.T) {
	got := (SendRegistrationNoticeArgs{}).InsertOpts().UniqueOpts
	want := river.UniqueOpts{ByArgs: true, ByPeriod: time.Hour}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unique opts = %#v, want %#v", got, want)
	}
}

func TestSendRegistrationNoticeWorker(t *testing.T) {
	mailer := &mockMailer{}
	w := NewSendRegistrationNoticeWorker(mailer)
	job := &river.Job[SendRegistrationNoticeArgs]{Args: SendRegistrationNoticeArgs{Email: "alice@example.com"}}

	if err := w.Work(t.Context(), job); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if !mailer.called {
		t.Fatal("mailer should be invoked")
	}
	if mailer.to != "alice@example.com" {
		t.Fatalf("recipient = %q, want alice@example.com", mailer.to)
	}
	if mailer.subject != "SimpleBank registration attempt" {
		t.Fatalf("subject = %q, want SimpleBank registration attempt", mailer.subject)
	}
	if mailer.msg != registrationNoticeBody {
		t.Fatalf("body = %q, want %q", mailer.msg, registrationNoticeBody)
	}
	for _, attackerField := range []string{"alice@example.com", "<script>", "full_name", "username"} {
		if strings.Contains(mailer.msg, attackerField) {
			t.Fatalf("body leaked attacker-controlled field %q: %q", attackerField, mailer.msg)
		}
	}
}
