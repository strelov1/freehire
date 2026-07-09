package search

import "testing"

// CurrentEmbedderModel is the single source of truth for the embedder identity: it is
// stamped on a job when embedded and used as the semantic-outbox staleness key, and it
// gates CV-vector staleness (handler/recommendations). An empty value would make every
// job look stale forever (endless re-embedding) and would silently invalidate every CV
// vector, so pin it: non-empty and equal to the declared model.
func TestCurrentEmbedderModel(t *testing.T) {
	if CurrentEmbedderModel() == "" {
		t.Fatal("CurrentEmbedderModel() is empty — staleness checks would never converge")
	}
	if CurrentEmbedderModel() != embedderModel {
		t.Errorf("CurrentEmbedderModel() = %q, want the embedderModel const %q", CurrentEmbedderModel(), embedderModel)
	}
}
