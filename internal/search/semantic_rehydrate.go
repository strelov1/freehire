package search

// semanticDocsFromPG wraps each document that carries a persisted vector as a
// userProvided semantic document, skipping any without one. It is the rehydrate path:
// a --from-pg rebuild reuses the vectors already stored in Postgres instead of
// re-embedding via TEI. A document with no persisted vector is left out of the rebuild
// (the embed worker fills it incrementally) rather than indexed without a vector, which
// the userProvided embedder rejects.
func semanticDocsFromPG(docs []JobDocument) []semanticDocument {
	out := make([]semanticDocument, 0, len(docs))
	for _, d := range docs {
		if len(d.semanticVector) == 0 {
			continue
		}
		out = append(out, semanticDocument{
			JobDocument: d,
			Vectors:     map[string][]float32{embedderName: d.semanticVector},
		})
	}
	return out
}
