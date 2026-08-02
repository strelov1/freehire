package applyform

import "testing"

// The enqueue gate and the fetcher registry must name the same providers. They are
// separate concerns — one runs during ingest, the other in the worker — and if they drift
// the queue fills with work nothing can drain, or a provider is silently never captured.
func TestCaptureProvidersMatchTheFetcherRegistry(t *testing.T) {
	fetchers := Fetchers(nil)

	for provider := range fetchers {
		if !NeedsRequestCapture(provider) {
			t.Errorf("provider %q has a fetcher but is never queued", provider)
		}
	}
	for _, provider := range []string{"greenhouse", "ashby", "workable", "lever"} {
		if !NeedsRequestCapture(provider) {
			t.Errorf("NeedsRequestCapture(%q) = false, want true", provider)
		}
		if _, ok := fetchers[provider]; !ok {
			t.Errorf("provider %q is queued but has no fetcher to drain it", provider)
		}
	}
}

// Recruitee's form rides along with the crawl, so queueing it would fetch a second time
// what ingest already holds.
func TestRecruiteeIsNeverQueued(t *testing.T) {
	if NeedsRequestCapture("recruitee") {
		t.Error("recruitee queued for a fetch, want it captured during ingest instead")
	}
}

// Every other provider — Workday behind its candidate session, SmartRecruiters behind
// DataDome, and the hundred-odd platforms nobody has looked at — must not accumulate queue
// entries that can never be drained.
func TestProvidersWithoutAReadableFormAreNeverQueued(t *testing.T) {
	for _, provider := range []string{"workday", "smartrecruiters", "oracle", "ukg", "telegram", ""} {
		if NeedsRequestCapture(provider) {
			t.Errorf("NeedsRequestCapture(%q) = true, want false — nothing can drain it", provider)
		}
	}
}
