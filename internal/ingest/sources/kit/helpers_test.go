package kit

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func TestCountryFromCode(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"alpha-2 lowercase", "ae", []string{"ae"}},
		{"alpha-2 uppercase", "AE", []string{"ae"}},
		{"alpha-3 as Ashby sends it", "USA", []string{"us"}},
		{"empty", "", nil},
		{"unresolved code outside the curated set", "ZZZ", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CountryFromCode(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("CountryFromCode(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// A detail request that failed tells the crawl one of two very different things, and the whole
// stale-job sweep hangs off which: 404/410 is the platform stating the posting is gone, and
// anything else is this crawl having failed to see a posting that may well still be there.
func TestDetailUnreadable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"the platform reports the posting gone", &StatusError{Code: 404}, false},
		{"the platform reports the posting withdrawn", &StatusError{Code: 410}, false},
		{"the origin refuses this address", &StatusError{Code: 403}, true},
		{"rate limited past the client's own retries", &StatusError{Code: 429}, true},
		{"the origin is broken", &StatusError{Code: 502}, true},
		{"an unexpected status says nothing about the posting", &StatusError{Code: 401}, true},
		{"a transport failure", errors.New("dial tcp: i/o timeout"), true},
		{"a status wrapped by an adapter", fmt.Errorf("detail: %w", &StatusError{Code: 404}), false},
		{"a decode failure", fmt.Errorf("sources: decode %s: %w", "https://x/y", errors.New("EOF")), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DetailUnreadable(tc.err); got != tc.want {
				t.Errorf("DetailUnreadable(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
