package sources

import (
	"net/http"
	"testing"
)

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
	for name, tc := range map[string]struct {
		rotating, fixed, wantProxy string
		freshExit                  bool
	}{
		"rotating": {"http://u:p@rotating.example:8000", "", "http://u:p@rotating.example:8000", true},
		"fixed":    {"", "http://u:p@fixed.example:8000", "http://u:p@fixed.example:8000", false},
		"both":     {"http://u:p@rotating.example:8000", "http://u:p@fixed.example:8000", "http://u:p@rotating.example:8000", true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("SOURCES_ROTATING_PROXY_URL", tc.rotating)
			t.Setenv("SOURCES_PROXY_URL", tc.fixed)

			c, proxied, err := NewFormClientFromEnv()
			if err != nil {
				t.Fatalf("NewFormClientFromEnv: %v", err)
			}
			if !proxied {
				t.Error("not proxied despite a configured proxy")
			}
			// The gateway hands out a new exit per connection, so only a client that closes
			// each one gets a fresh exit per request.
			tr := c.httpClient.Transport.(*http.Transport)
			if tr.DisableKeepAlives != tc.freshExit {
				t.Errorf("DisableKeepAlives = %v, want %v", tr.DisableKeepAlives, tc.freshExit)
			}
			if tr.Proxy == nil {
				t.Fatal("transport has no proxy")
			}
			req, err := http.NewRequest(http.MethodGet, "https://apply.workable.com/", nil)
			if err != nil {
				t.Fatal(err)
			}
			got, err := tr.Proxy(req)
			if err != nil || got == nil {
				t.Fatalf("proxy = %v, error = %v", got, err)
			}
			if got.String() != tc.wantProxy {
				t.Errorf("proxy = %s, want %s", got, tc.wantProxy)
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
