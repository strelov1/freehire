package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/search"
)

// vectorSource yields one offset-paged window of persisted vectors from the semantic
// index. An empty page means the end: every jobs_semantic document carries a vector,
// so a window past the last document returns nothing.
type vectorSource interface {
	Page(ctx context.Context, offset, limit int) ([]search.SemanticVector, error)
}

// vectorSink persists a batch of (id, vector) pairs into Postgres and reports how many
// rows it wrote. A no-op counting sink implements the same port for --dry-run.
type vectorSink interface {
	Save(ctx context.Context, vecs []search.SemanticVector) (int64, error)
}

// seedStats reports what a backfill run moved.
type seedStats struct {
	Pages   int
	Fetched int
	Saved   int64
}

// seeder copies vectors Meili→Postgres a page at a time. The offset advances by
// pageSize (not by the number of vectors returned) so parser-skipped documents never
// stall the walk.
type seeder struct {
	src      vectorSource
	sink     vectorSink
	pageSize int
}

func (s seeder) run(ctx context.Context) (seedStats, error) {
	var st seedStats
	for offset := 0; ; offset += s.pageSize {
		page, err := s.src.Page(ctx, offset, s.pageSize)
		if err != nil {
			return st, err
		}
		if len(page) == 0 {
			return st, nil
		}
		saved, err := s.sink.Save(ctx, page)
		if err != nil {
			return st, err
		}
		st.Pages++
		st.Fetched += len(page)
		st.Saved += saved
		if st.Pages%50 == 0 {
			log.Printf("backfill-semantic-vectors: progress pages=%d fetched=%d saved=%d", st.Pages, st.Fetched, st.Saved)
		}
	}
}
