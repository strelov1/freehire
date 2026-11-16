package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"

	"github.com/strelov1/freehire/internal/ingest/boardcatalog"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

type registrationStats struct {
	Planned, Added, AlreadyKnown, Rejected int
}

type searchInserter interface {
	Insert(context.Context, boardcatalog.InsertInput, boardcatalog.Status) (boardcatalog.Board, error)
}

// registrationPlan validates every search before opening the database or writing
// the first row. These are search slices, not employer identities.
func registrationPlan(queries []query) ([]boardcatalog.InsertInput, error) {
	queries, err := normalizeQueries(queries)
	if err != nil {
		return nil, err
	}
	registry := sources.Taxonomy()
	inputs := make([]boardcatalog.InsertInput, 0, len(queries))
	for _, q := range queries {
		in := boardcatalog.InsertInput{
			Provider: "linkedin", Board: q.Keywords, Region: q.Location,
			Company: "LinkedIn " + q.Location + " " + q.Keywords,
			Tenants: map[string]string{"pages": strconv.Itoa(q.Pages), "jobage": strconv.Itoa(q.JobAge)},
		}
		if err := boardcatalog.Validate(in, registry); err != nil {
			return nil, err
		}
		inputs = append(inputs, in)
	}
	return inputs, nil
}

func runRegisterSearches(queries []query, apply bool) int {
	inputs, err := registrationPlan(queries)
	if err != nil {
		log.Printf("harvest-linkedin: search registration: %v", err)
		return 1
	}
	if !apply {
		for _, in := range inputs {
			log.Printf("harvest-linkedin: would register keyword=%q location=%q pages=%s jobage=%s status=pending",
				in.Board, in.Region, in.Tenants["pages"], in.Tenants["jobage"])
		}
		log.Printf("harvest-linkedin: preview planned=%d added=0 rejected=0; existing catalogue not read; pass --apply to register", len(inputs))
		return 0
	}
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("harvest-linkedin: database: %v", err)
		return 1
	}
	defer cleanup()
	insert := boardcatalog.NewInserter(boardcatalog.NewQueriesRepository(db.New(pool)), sources.Taxonomy())
	st, err := registerSearches(ctx, insert, inputs)
	log.Printf("harvest-linkedin: registration planned=%d added=%d already_known=%d rejected=%d", st.Planned, st.Added, st.AlreadyKnown, st.Rejected)
	if err != nil {
		log.Printf("harvest-linkedin: register: %v", err)
		return 1
	}
	return 0
}

func registerSearches(ctx context.Context, insert searchInserter, inputs []boardcatalog.InsertInput) (registrationStats, error) {
	st := registrationStats{Planned: len(inputs)}
	for _, in := range inputs {
		b, err := insert.Insert(ctx, in, boardcatalog.StatusPending)
		if errors.Is(err, boardcatalog.ErrDuplicateBoard) {
			st.AlreadyKnown++
			log.Printf("harvest-linkedin: search already registered keyword=%q location=%q; configuration unchanged", in.Board, in.Region)
			continue
		}
		if err != nil {
			return st, fmt.Errorf("keyword=%q location=%q: %w", in.Board, in.Region, err)
		}
		if b.Status == boardcatalog.StatusRejected {
			st.Rejected++
			return st, fmt.Errorf("keyword=%q location=%q rejected: %s", in.Board, in.Region, b.RejectedReason)
		}
		st.Added++
		log.Printf("harvest-linkedin: registered search id=%d keyword=%q location=%q status=%s", b.ID, in.Board, in.Region, b.Status)
	}
	return st, nil
}
