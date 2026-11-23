package sources

import (
	"context"
	"errors"
	"testing"
)

const oracleSites = `{"hasMore": false, "items": [
	{"SiteNumber": "CX_1", "SiteCode": "CX", "StatusCode": "ORA_ACTIVE"},
	{"SiteNumber": "CX_9", "SiteCode": "OLD", "StatusCode": "ORA_INACTIVE"}]}`

func TestOracleCanonicalBoard(t *testing.T) {
	const host = "acme.fa.us2.oraclecloud.com"
	cases := []struct {
		name, board, want string
		http              *routedHTTP
		wantErr           bool
	}{
		{"listed active site", host + "/CX_1", host + "/CX_1", (&routedHTTP{}).route("recruitingCESites", oracleSites), false},
		{"case mismatch maps to listed spelling", host + "/cx_1", host + "/CX_1", (&routedHTTP{}).route("recruitingCESites", oracleSites), false},
		{"unlisted site refused", host + "/jobsearch", "", (&routedHTTP{}).route("recruitingCESites", oracleSites), true},
		{"inactive site refused", host + "/CX_9", "", (&routedHTTP{}).route("recruitingCESites", oracleSites), true},
		{"fetch failure does not block", host + "/jobsearch", host + "/jobsearch", (&routedHTTP{}).routeErr("recruitingCESites", errors.New("boom")), false},
		{"truncated list does not block", host + "/Z", host + "/Z",
			(&routedHTTP{}).route("recruitingCESites", `{"hasMore": true, "items": [{"SiteNumber": "CX_1", "StatusCode": "ORA_ACTIVE"}]}`), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := NewOracle(c.http).(BoardCanonicalizer).CanonicalBoard(context.Background(), c.board)
			if (err != nil) != c.wantErr || got != c.want {
				t.Errorf("CanonicalBoard(%q) = %q, %v; want %q, err=%v", c.board, got, err, c.want, c.wantErr)
			}
		})
	}
}

func TestOracleCanonicalBoardWithoutTransportDoesNotBlock(t *testing.T) {
	got, err := NewOracle(nil).(BoardCanonicalizer).CanonicalBoard(context.Background(), "a.fa.us2.oraclecloud.com/X")
	if err != nil || got != "a.fa.us2.oraclecloud.com/X" {
		t.Errorf("got %q, %v", got, err)
	}
}
