package logging

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestRedactLogValueRedactsSensitiveKeys(t *testing.T) {
	cases := []string{
		"authorization",
		"agent_token",
		"client_secret",
		"database_dsn",
		"private_key",
	}
	for _, key := range cases {
		if got := redactLogValue(key, "secret-value"); got != "<redacted>" {
			t.Fatalf("expected %s to be redacted, got %v", key, got)
		}
	}
	if got := redactLogValue("server_url", "https://cdn.example.com"); got != "https://cdn.example.com" {
		t.Fatalf("expected non-sensitive key to pass through, got %v", got)
	}
}

func TestCustomTextHandlerRedactsMessageText(t *testing.T) {
	var output bytes.Buffer
	handler := &customTextHandler{writer: &output, level: slog.LevelDebug}
	record := slog.NewRecord(time.Now(), slog.LevelError, `failed Authorization=Bearer raw-token password="raw-password"`, 0)

	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	text := output.String()
	for _, leaked := range []string{"raw-token", "raw-password"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("expected log message to redact %q, got %q", leaked, text)
		}
	}
	if !strings.Contains(text, "<redacted>") {
		t.Fatalf("expected redaction marker in log message, got %q", text)
	}
}
