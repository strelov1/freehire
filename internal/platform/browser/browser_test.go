package browser

import (
	"fmt"
	"strings"
	"testing"
)

// The stealth pair is the whole point of this package existing rather than each caller
// spelling the flags out, so the test names them rather than counting them.
func TestLaunchFlagsCarryTheStealthPair(t *testing.T) {
	got, err := launchFlags("")
	if err != nil {
		t.Fatalf("launchFlags: %v", err)
	}
	for _, want := range []flag{
		{name: "headless", value: true},
		{name: "disable-blink-features", value: "AutomationControlled"},
	} {
		if !hasFlag(got, want) {
			t.Errorf("missing %s=%v in %v", want.name, want.value, got)
		}
	}
}

func TestLaunchFlagsAddNoProxyWhenNoneGiven(t *testing.T) {
	got, err := launchFlags("")
	if err != nil {
		t.Fatalf("launchFlags: %v", err)
	}
	for _, f := range got {
		if f.name == "proxy-server" {
			t.Errorf("proxy-server=%v set with no proxy configured", f.value)
		}
	}
}

// The credential check is the reason this function is inspectable at all. Chrome takes its
// proxy as a command-line argument, and a command line is readable by every process on the
// host — so the scheme and host may go there and the credentials may not. They are answered
// over the debugging protocol instead (see Session).
func TestLaunchFlagsKeepProxyCredentialsOutOfTheCommandLine(t *testing.T) {
	got, err := launchFlags("http://bob:hunter2@proxy.example:8080")
	if err != nil {
		t.Fatalf("launchFlags: %v", err)
	}
	if !hasFlag(got, flag{name: "proxy-server", value: "http://proxy.example:8080"}) {
		t.Errorf("proxy-server flag missing or wrong, got %v", got)
	}
	for _, f := range got {
		s := rendered(f)
		for _, secret := range []string{"bob", "hunter2"} {
			if strings.Contains(s, secret) {
				t.Errorf("credential %q leaked into flag %s=%v", secret, f.name, f.value)
			}
		}
	}
}

func TestProxyCredentialsAreReadableForTheProtocolPath(t *testing.T) {
	user, pass, err := proxyCredentials("http://bob:hunter2@proxy.example:8080")
	if err != nil {
		t.Fatalf("proxyCredentials: %v", err)
	}
	if user != "bob" || pass != "hunter2" {
		t.Errorf("got %q/%q, want bob/hunter2", user, pass)
	}
}

func TestProxyCredentialsAreEmptyForAnUnauthenticatedProxy(t *testing.T) {
	user, pass, err := proxyCredentials("http://proxy.example:8080")
	if err != nil {
		t.Fatalf("proxyCredentials: %v", err)
	}
	if user != "" || pass != "" {
		t.Errorf("got %q/%q, want both empty", user, pass)
	}
}

// A proxy URL that cannot be parsed fails the launch rather than silently starting a browser
// on the direct IP — which, for the one provider that needs this, would look exactly like a
// working crawl that finds nothing.
func TestLaunchFlagsRejectAnUnparseableProxy(t *testing.T) {
	if _, err := launchFlags("://not a url"); err == nil {
		t.Error("want an error for an unparseable proxy URL, got nil")
	}
}

func hasFlag(flags []flag, want flag) bool {
	for _, f := range flags {
		if f.name == want.name && f.value == want.value {
			return true
		}
	}
	return false
}

func rendered(f flag) string {
	return fmt.Sprintf("%s=%v", f.name, f.value)
}

// An error about a malformed proxy must not be the thing that prints the password into a log.
func TestProxyIsRedactedInErrorText(t *testing.T) {
	_, err := launchFlags("http://bob:hunter2@")
	if err == nil {
		t.Fatal("want an error for a proxy URL with no host")
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Errorf("password leaked into the error text: %v", err)
	}
}

// LaunchOptions and LaunchOptionsThroughProxy("") must decide the same thing — they are one
// launch with and without a proxy, not two. They reach chromedp by different lines, so the
// flag set they share is what this pins; option values are opaque funcs and cannot be compared.
func TestDirectLaunchDecidesExactlyTheStealthFlags(t *testing.T) {
	viaProxyPath, err := launchFlags("")
	if err != nil {
		t.Fatalf("launchFlags: %v", err)
	}
	direct := stealthFlags()
	if len(viaProxyPath) != len(direct) {
		t.Fatalf("launchFlags(\"\") has %d flags, stealthFlags has %d", len(viaProxyPath), len(direct))
	}
	for _, want := range direct {
		if !hasFlag(viaProxyPath, want) {
			t.Errorf("launchFlags(\"\") is missing %s", rendered(want))
		}
	}
}
