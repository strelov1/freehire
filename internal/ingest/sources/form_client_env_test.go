package sources

import "testing"

// With no proxy configured the caller is told so, rather than handed a direct client it
// would take for a proxied one.
func TestNewFormClientFromEnv_NoProxyIsReported(t *testing.T) {
	t.Setenv("SOURCES_ROTATING_PROXY_URL", "")
	t.Setenv("SOURCES_PROXY_URL", "")

	_, proxied, err := NewFormClientFromEnv()
	if err != nil {
		t.Fatalf("NewFormClientFromEnv: %v", err)
	}
	if proxied {
		t.Error("reported proxied with no proxy configured")
	}
}

// Either proxy carries every request; the rotating gateway wins when both are set, since a
// fresh exit per request is what keeps any one address from being banned.
func TestNewFormClientFromEnv_EitherProxyCarriesEveryRequest(t *testing.T) {
	for name, env := range map[string][2]string{
		"rotating": {"http://u:p@rotating.example:8000", ""},
		"fixed":    {"", "http://u:p@fixed.example:8000"},
		"both":     {"http://u:p@rotating.example:8000", "http://u:p@fixed.example:8000"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("SOURCES_ROTATING_PROXY_URL", env[0])
			t.Setenv("SOURCES_PROXY_URL", env[1])

			_, proxied, err := NewFormClientFromEnv()
			if err != nil {
				t.Fatalf("NewFormClientFromEnv: %v", err)
			}
			if !proxied {
				t.Error("not proxied despite a configured proxy")
			}
		})
	}
}

// A set but unreadable value fails the worker at startup rather than quietly sending it out
// from the host's own address.
func TestNewFormClientFromEnv_AnUnreadableProxyFails(t *testing.T) {
	t.Setenv("SOURCES_ROTATING_PROXY_URL", "")
	t.Setenv("SOURCES_PROXY_URL", "::not a url")

	if _, _, err := NewFormClientFromEnv(); err == nil {
		t.Error("an unparseable SOURCES_PROXY_URL was accepted")
	}
}
