package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// egressFor is the client a harvest probes through. By default it is the provider's own egress
// policy (sources.ClientFor), the one the crawl uses. -rotating sends every probe through the
// rotating gateway instead, a fresh exit IP per request. A harvest is a burst the crawl never
// is: a refusal-retry provider tries its DIRECT IP first, and a platform that bans by IP answers
// each first try with a 429 that renews the ban. Skipping the direct lane is the only way a bulk
// probe stops feeding the ban it is waiting out.
func egressFor(provider string, rotating bool) (*sources.Client, error) {
	if !rotating {
		return sources.ClientFor(provider)
	}
	raw := strings.TrimSpace(os.Getenv("SOURCES_ROTATING_PROXY_URL"))
	if raw == "" {
		return nil, errors.New("-rotating needs SOURCES_ROTATING_PROXY_URL")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		// Never print the value: it carries the proxy credential.
		return nil, fmt.Errorf("-rotating: SOURCES_ROTATING_PROXY_URL is not a proxy URL")
	}
	return sources.NewRotatingProxyClient(u), nil
}

// withProberClient lets a prober that needs a client of its own (probeClienter) replace the
// egress. That client knows nothing of the rotating gateway, so under -rotating the swap would
// quietly send the probe somewhere the operator did not ask for; it is refused instead.
func withProberClient(p prober, egress *sources.Client, rotating bool) (*sources.Client, error) {
	pc, ok := p.(probeClienter)
	if !ok {
		return egress, nil
	}
	c := pc.probeClient()
	if c == nil {
		return egress, nil
	}
	if rotating {
		return nil, errors.New("-rotating is not supported: this prober probes through a client of its own")
	}
	return c, nil
}
