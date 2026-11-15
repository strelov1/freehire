// Package embedprogress measures how much of the catalogue carries a current semantic
// embedding and how fast the rest is being covered, for the public /embeddings page.
//
// Two measurements are combined at read time. Coverage (eligible and covered postings) is a slow
// scan, so cmd/rollup-stats publishes it as a Snapshot and no request path counts it. The claimable
// depth of semantic_outbox is a cheap read: cmd/server samples it every 15 seconds (Sampler) and keeps
// ten minutes in memory (Ring), which gives the rate. The snapshot records the depth it was taken
// at, so the live covered figure is the measured count plus how much the queue has drained since
// (see Build for why not eligible - queued).
//
// It sits in the search block because the coverage predicate uses search.CurrentEmbedderModel(),
// which ai (where internal/ai/embed lives) cannot see.
package embedprogress
