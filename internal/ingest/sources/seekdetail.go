package sources

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
)

// seekDetailNetwork is the one GraphQL jobDetails client the whole SEEK network shares: seek
// (AU/NZ) and jobstreet (JobStreet/JobsDB) use the same infrastructure, so a ban on one is a ban
// on both. It is built once per registry and handed to both adapters.
//
// It owns three run-scoped decisions no single board can make:
//
//   - which egress LANE a detail request takes: the direct IP first, then, after the first
//     RATE_LIMITED refusal on it, the proxy, which has its own pacer (see seekProxyDetailInterval);
//   - when to STOP hydrating: a refusal on the proxy lane too, or on the direct lane with no
//     proxy configured, ends detail traffic for the rest of the run;
//   - how many requests the run may SPEND at all (budget, see seekDetailRunBudget).
//
// A posting that gets no body because the network stopped is deferred like a failed detail (seek
// never stores a posting body-less), so a stopped run costs latency, not data.
//
// State is per process and cmd/ingest crawls one provider per process, so nothing carries a ban
// across runs: every run re-probes the direct IP, and the budget bounds what that probe costs.
type seekDetailNetwork struct {
	direct JSONPoster // paced (pacedSeekPoster)
	proxy  JSONPoster // paced on its own bucket (pacedSeekProxyPoster); nil = no proxy configured
	budget int

	mu   sync.Mutex
	lane seekLane
	sent [seekLaneStopped]int // requests sent per lane
}

// seekLane is the egress a detail request takes; seekLaneStopped means none.
type seekLane int

const (
	seekLaneDirect seekLane = iota
	seekLaneProxy
	seekLaneStopped
)

func (l seekLane) String() string {
	switch l {
	case seekLaneDirect:
		return "direct"
	case seekLaneProxy:
		return "proxy"
	default:
		return "stopped"
	}
}

// seekDetailRunBudget caps the jobDetails requests one run may send across the whole network,
// both lanes counted. SEEK bans on cumulative volume, not rate, so this is a budget rather than a
// pacer. Postings past the cap are deferred to the next run, so a large bootstrap accretes over
// several runs.
const seekDetailRunBudget = 1500

// newSeekDetailNetwork wires the shared detail client. direct and proxy must already be paced; a
// nil proxy means the proxy lane does not exist and the first refusal stops hydration.
func newSeekDetailNetwork(direct, proxy JSONPoster) *seekDetailNetwork {
	return &seekDetailNetwork{direct: direct, proxy: proxy, budget: seekDetailRunBudget}
}

// seekDetailOutcome tells hydrate what happened to one posting's detail. Only detailFailed is a
// per-posting event worth logging: a refusal and a stop are network-wide and logged once.
type seekDetailOutcome int

const (
	detailHydrated seekDetailOutcome = iota
	detailFailed                     // transport error, non-refusal GraphQL error, or empty content
	detailStopped                    // no request sent: the network stopped or the budget is spent
)

// seekDetailQuery is the description-fetching GraphQL operation. Its variables are declared exactly
// as used: SEEK rejects an unused variable with GRAPHQL_VALIDATION_FAILED, so a drifted field fails
// loudly instead of silently emptying every body.
const seekDetailQuery = `query jobDetails($jobId: ID!) {
  jobDetails(id: $jobId) {
    job {
      content(platform: WEB)
    }
  }
}`

// seekDetailResponse is the slice of the GraphQL reply the adapter reads. Errors matters because
// SEEK's rate limit is NOT an HTTP status: a refused jobDetails answers 200 with
// {"errors":[{"message":"Too many requests.","extensions":{"code":"RATE_LIMITED"}}],
// "data":{"jobDetails":null}}, which without this field reads as a posting with no content.
type seekDetailResponse struct {
	Errors []struct {
		Extensions struct {
			Code string `json:"code"`
		} `json:"extensions"`
	} `json:"errors"`
	Data struct {
		JobDetails struct {
			Job struct {
				Content string `json:"content"`
			} `json:"job"`
		} `json:"jobDetails"`
	} `json:"data"`
}

// rateLimited reports whether SEEK refused the request for volume rather than answering it.
func (r seekDetailResponse) rateLimited() bool {
	for _, e := range r.Errors {
		if e.Extensions.Code == "RATE_LIMITED" {
			return true
		}
	}
	return false
}

// content fetches one posting's description (raw HTML, unsanitized) from host's GraphQL endpoint.
// A refusal is never returned to the caller as such: it moves the network to its next lane and the
// same posting is retried there, so the caller only ever sees a body, a failure, or a stop.
// provider only labels the log line a lane change emits.
func (n *seekDetailNetwork) content(ctx context.Context, provider, host, id string) (string, seekDetailOutcome) {
	body := map[string]any{
		"operationName": "jobDetails",
		"query":         seekDetailQuery,
		"variables":     map[string]any{"jobId": id},
	}
	for {
		lane, poster, ok := n.acquire(provider)
		if !ok {
			return "", detailStopped
		}
		var resp seekDetailResponse
		err := poster.PostJSON(ctx, host+"/graphql", body, &resp)
		if seekRefused(err, resp) {
			n.refused(provider, lane)
			continue // the lane moved on (or another worker already moved it): try the next one
		}
		content := resp.Data.JobDetails.Job.Content
		if err != nil || strings.TrimSpace(content) == "" {
			return "", detailFailed
		}
		return content, detailHydrated
	}
}

// seekRefused reports a volume refusal: the GraphQL RATE_LIMITED or an HTTP 429.
func seekRefused(err error, resp seekDetailResponse) bool {
	var se *StatusError
	if errors.As(err, &se) {
		return se.Code == http.StatusTooManyRequests
	}
	return err == nil && resp.rateLimited()
}

// acquire charges one request to the run's budget and returns the lane to send it on, or ok=false
// when the network has stopped or the budget is spent.
func (n *seekDetailNetwork) acquire(provider string) (seekLane, JSONPoster, bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.lane == seekLaneStopped {
		return seekLaneStopped, nil, false
	}
	if n.total() >= n.budget {
		n.stop(provider, "the run's detail budget is spent")
		return seekLaneStopped, nil, false
	}
	n.sent[n.lane]++
	if n.lane == seekLaneProxy {
		return n.lane, n.proxy, true
	}
	return n.lane, n.direct, true
}

// refused advances the network past lane after a refusal on it. It is a no-op when lane is no
// longer the current one: requests already in flight on the direct lane when the switch happened
// come back refused too, and must not count against the proxy lane they never touched.
func (n *seekDetailNetwork) refused(provider string, lane seekLane) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if lane != n.lane {
		return
	}
	switch {
	case lane == seekLaneDirect && n.proxy != nil:
		n.lane = seekLaneProxy
		log.Printf("%s: SEEK jobDetails RATE_LIMITED on the direct IP after %d detail requests this run; "+
			"switching the rest of the run's detail requests to the proxy lane", provider, n.sent[seekLaneDirect])
	case lane == seekLaneDirect:
		n.stop(provider, "jobDetails RATE_LIMITED on the direct IP and no proxy is configured")
	default:
		n.stop(provider, "jobDetails RATE_LIMITED on the proxy lane too")
	}
}

// stop ends hydration for the rest of the run and logs it. Callers hold mu and reach it only from
// a live lane, so the line is written once per run.
func (n *seekDetailNetwork) stop(provider, why string) {
	n.lane = seekLaneStopped
	log.Printf("%s: SEEK detail hydration stopped for the rest of the run (%s) after %d requests "+
		"(direct %d, proxy %d); every posting still lacking a body is deferred to the next run",
		provider, why, n.total(), n.sent[seekLaneDirect], n.sent[seekLaneProxy])
}

func (n *seekDetailNetwork) total() int { return n.sent[seekLaneDirect] + n.sent[seekLaneProxy] }
