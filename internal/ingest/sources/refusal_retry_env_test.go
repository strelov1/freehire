package sources

import "testing"

// With no proxy configured the form fetchers keep today's direct-only client: a refused
// request has nowhere else to go, and nothing pretends otherwise.
func TestNewRefusalRetryClientFromEnv_NoProxyIsDirectOnly(t *testing.T) {
	t.Setenv("SOURCES_ROTATING_PROXY_URL", "")
	t.Setenv("SOURCES_PROXY_URL", "")

	c, err := NewRefusalRetryClientFromEnv()
	if err != nil {
		t.Fatalf("NewRefusalRetryClientFromEnv: %v", err)
	}
	if c.refusalClient != nil {
		t.Error("a client with no proxy configured has a refusal lane")
	}
}

// Either proxy gives a refused request a second lane; the rotating gateway is the one that
// answers Workable's per-IP ban.
func TestNewRefusalRetryClientFromEnv_EitherProxyAddsTheRefusalLane(t *testing.T) {
	for name, env := range map[string][2]string{
		"rotating": {"http://u:p@rotating.example:8000", ""},
		"fixed":    {"", "http://u:p@fixed.example:8000"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("SOURCES_ROTATING_PROXY_URL", env[0])
			t.Setenv("SOURCES_PROXY_URL", env[1])

			c, err := NewRefusalRetryClientFromEnv()
			if err != nil {
				t.Fatalf("NewRefusalRetryClientFromEnv: %v", err)
			}
			if c.refusalClient == nil {
				t.Error("no refusal lane despite a configured proxy")
			}
		})
	}
}

// A set but unreadable value fails the worker at startup rather than quietly leaving it on
// the banned address.
func TestNewRefusalRetryClientFromEnv_AnUnreadableProxyFails(t *testing.T) {
	t.Setenv("SOURCES_ROTATING_PROXY_URL", "")
	t.Setenv("SOURCES_PROXY_URL", "::not a url")

	if _, err := NewRefusalRetryClientFromEnv(); err == nil {
		t.Error("an unparseable SOURCES_PROXY_URL was accepted")
	}
}
