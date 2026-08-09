package search

import "github.com/jackc/pgx/v5/pgtype"

// SemanticChunksFromArray reshapes jobs.semantic_embedding into its chunk vectors.
// Postgres does not enforce an array column's declared dimension count — a `real[]`
// column freely holds either a flat vector or an array of vectors (verified against a
// live Postgres 18 instance; see design.md Decision 3) — so a row embedded under the
// OLD single-vector scheme still scans cleanly here as a 1-dimension array, and is
// treated as a single chunk: exactly the vector this job carried before chunking
// existed. This is what lets every job-loading query in the codebase — not just the
// embed pipeline — keep working for a row that has not yet been re-embedded, for as
// long as that takes, with no data migration and no deploy-timing coordination.
//
// Only 1 and 2 dimensions are meaningful shapes for this column (the old single-vector
// write and the new per-chunk write, respectively — see SemanticChunksToArray, the only
// writer, which never produces anything else). A higher dimension is not a shape this
// column's writers can produce, so it is not handled — reshaping would silently read
// only the first "layer" of Elements rather than erroring.
func SemanticChunksFromArray(a pgtype.Array[float32]) [][]float32 {
	if !a.Valid || len(a.Dims) == 0 {
		return nil
	}
	if len(a.Dims) == 1 {
		return [][]float32{a.Elements}
	}
	rows, cols := int(a.Dims[0].Length), int(a.Dims[1].Length)
	out := make([][]float32, rows)
	for i := range out {
		out[i] = a.Elements[i*cols : (i+1)*cols]
	}
	return out
}

// SemanticChunksToArray is the write-side inverse of SemanticChunksFromArray: it
// flattens a job's chunk vectors into the (Elements, Dims) shape SetSemanticEmbedding
// binds as its parameter. An empty chunk list produces an invalid (NULL) array rather
// than a zero-dimension one, matching how "no vector yet" is represented on read.
func SemanticChunksToArray(chunks [][]float32) pgtype.Array[float32] {
	if len(chunks) == 0 {
		return pgtype.Array[float32]{}
	}
	cols := len(chunks[0])
	elements := make([]float32, 0, len(chunks)*cols)
	for _, c := range chunks {
		elements = append(elements, c...)
	}
	return pgtype.Array[float32]{
		Elements: elements,
		Dims: []pgtype.ArrayDimension{
			{Length: int32(len(chunks)), LowerBound: 1},
			{Length: int32(cols), LowerBound: 1},
		},
		Valid: true,
	}
}
