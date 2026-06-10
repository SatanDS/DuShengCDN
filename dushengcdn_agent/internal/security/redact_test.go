package security

import (
	"strings"
	"testing"
)

func TestRedactSensitiveTextScrubsConfigAndCredentials(t *testing.T) {
	input := `openresty -t failed:
local expected_hash = "abcdef123456"
proxy_set_header Authorization "Bearer secret-token";
proxy_set_header Cookie "session=abc";
error Authorization=Basic dXNlcjpwYXNz token=raw-token password="raw-password"
GET /callback?code=oauth-code&state=csrf-state&safe=1`

	redacted := RedactSensitiveText(input)
	for _, leaked := range []string{
		"abcdef123456",
		"secret-token",
		"session=abc",
		"dXNlcjpwYXNz",
		"raw-token",
		"raw-password",
		"oauth-code",
		"csrf-state",
	} {
		if strings.Contains(redacted, leaked) {
			t.Fatalf("expected %q to be redacted from %q", leaked, redacted)
		}
	}
	if !strings.Contains(redacted, "safe=1") {
		t.Fatalf("expected non-sensitive query parameter to remain, got %q", redacted)
	}
}
