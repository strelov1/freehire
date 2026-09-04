//go:build integration

package autoapplyorchestrate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/inngest/inngestgo"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// inngestImage is pinned to the exact version github.com/inngest/inngestgo in go.mod was
// developed against, matching this repo's "always pin an exact version" convention.
const inngestImage = "inngest/inngest:v1.19.3"

// devServer is a real, freshly booted Inngest dev server plus the SDK-side HTTP server
// (also real, in-process) that Register's function is served from. One per test: cheap
// enough (a few seconds) and avoids any cross-test state via the dev server's own
// function registry.
type devServer struct {
	eventAPIBaseURL string
}

// startDevServer boots a real Inngest dev server (testcontainers) and a real in-process
// SDK server carrying the function Register creates against cfg, then drives the exact
// self-registration handshake this package's own spike verified by hand: PUT the SDK
// server's own endpoint to trigger an out-of-band sync to the dev server, then poll the
// dev server's own introspection endpoint until the function appears.
func startDevServer(t *testing.T, cfg Config) *devServer {
	t.Helper()
	ctx := context.Background()

	mux := http.NewServeMux()
	sdkServer := httptest.NewServer(mux)
	t.Cleanup(sdkServer.Close)
	sdkPort := sdkServer.Listener.Addr().(*net.TCPAddr).Port

	container, err := testcontainers.Run(ctx, inngestImage,
		testcontainers.WithCmd("inngest", "dev", "--host", "0.0.0.0", "--no-poll"),
		testcontainers.WithExposedPorts("8288/tcp"),
		testcontainers.WithHostPortAccess(sdkPort),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/dev").WithPort("8288/tcp")),
	)
	testcontainers.CleanupContainer(t, container)
	if err != nil {
		t.Fatalf("start inngest dev server: %v", err)
	}
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		// container.Logs returns a live-tailing stream that never reaches EOF for a
		// running container, so io.ReadAll on it would hang — `docker logs` (a
		// finite snapshot, not a follow) is what actually completes.
		out, lerr := exec.CommandContext(ctx, "docker", "logs", container.GetContainerID()).CombinedOutput()
		if lerr != nil {
			return
		}
		t.Logf("inngest dev server logs on failure:\n%s", out)
	})

	eventAPIBaseURL, err := container.PortEndpoint(ctx, "8288/tcp", "http")
	if err != nil {
		t.Fatalf("event API endpoint: %v", err)
	}

	sdkURL, err := url.Parse(fmt.Sprintf("http://%s:%d/api/inngest", testcontainers.HostInternal, sdkPort))
	if err != nil {
		t.Fatalf("parse sdk url: %v", err)
	}
	// RegisterURL must point at THIS test's own dev server container, not the
	// package-wide default (127.0.0.1:8288): testcontainers assigns an ephemeral host
	// port for the container's 8288, which only matches the default by coincidence.
	registerURL := eventAPIBaseURL + "/fn/register"
	dev := true
	client, err := inngestgo.NewClient(inngestgo.ClientOpts{
		AppID:       "freehire-auto-apply-orchestrator-test",
		Dev:         &dev,
		URL:         sdkURL,
		RegisterURL: &registerURL,
	})
	if err != nil {
		t.Fatalf("new inngest client: %v", err)
	}
	if _, err := Register(client, cfg); err != nil {
		t.Fatalf("register function: %v", err)
	}
	mux.Handle("/api/inngest", client.Serve())

	// Trigger self-registration: a PUT to our own SDK endpoint answers out-of-band by
	// POSTing its sync payload to the dev server (verified against a real dev server in
	// this session's own spike — see design.md's Context).
	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, sdkServer.URL+"/api/inngest", nil)
	if err != nil {
		t.Fatalf("build register request: %v", err)
	}
	putResp, err := http.DefaultClient.Do(putReq)
	if err != nil {
		t.Fatalf("trigger self-registration: %v", err)
	}
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("self-registration status = %d, want 200", putResp.StatusCode)
	}

	if !waitForFunctionRegisteredOK(eventAPIBaseURL, FunctionID, 10*time.Second) {
		t.Fatalf("function %q never appeared in the dev server's registry within 10s", FunctionID)
	}

	return &devServer{eventAPIBaseURL: eventAPIBaseURL}
}

// waitForFunctionRegisteredOK polls the dev server's own introspection endpoint until the
// named function appears in its registry — the out-of-band sync above completes
// asynchronously on the dev server side, so a fixed sleep would either race it or waste
// time; this stops the instant the sync is visible.
func waitForFunctionRegisteredOK(eventAPIBaseURL, functionID string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if devServerHasFunction(eventAPIBaseURL, functionID) {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func devServerHasFunction(eventAPIBaseURL, functionID string) bool {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, eventAPIBaseURL+"/dev", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var info struct {
		Functions []struct {
			Slug string `json:"slug"`
		} `json:"functions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return false
	}
	for _, f := range info.Functions {
		// The dev server slugs a function as "<appID>-<functionID>".
		if strings.HasSuffix(f.Slug, functionID) {
			return true
		}
	}
	return false
}

// sendEvent publishes one event to the dev server. The event key in the URL path is
// unchecked in dev mode (require_keys defaults to false) — any non-empty value works,
// matching this package's own verified spike.
func (d *devServer) sendEvent(t *testing.T, name string, data any) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"name": name, "data": data})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, d.eventAPIBaseURL+"/e/test", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build event request %s: %v", name, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send event %s: %v", name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("send event %s: status %d", name, resp.StatusCode)
	}
}
