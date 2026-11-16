package linkimport

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/ingest/atsboard"
	"github.com/strelov1/freehire/internal/ingest/linksource"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
)

// NativeSourcePendingReason withdraws an unverified discovery from the live catalogue.
// It preserves its identity so a verified ATS transfer can reopen the same posting.
const NativeSourcePendingReason = "native_source_pending"

type NativeDestination struct {
	Entry           sources.CompanyEntry
	Job             sources.Job
	EmployerWebsite string
}

type destinationCatalog struct {
	stored   linksource.BoardCatalog
	fallback sources.CompanyEntry
}

func (c destinationCatalog) Entry(ctx context.Context, source, board string) (sources.CompanyEntry, bool, error) {
	if c.stored != nil {
		e, found, err := c.stored.Entry(ctx, source, board)
		if err != nil || found {
			return e, found, err
		}
	}
	return c.fallback, true, nil
}

// ResolveNative retains verified board metadata on a later posting-read failure.
// A Job is available only when the returned error is nil.
func (im *Importer) ResolveNative(ctx context.Context, row db.Job, raw, employerWebsite string) (NativeDestination, error) {
	registry, catalog := im.ingest, im.catalog
	source, board, canonical, ok := atsboard.Recognize(raw)
	if !ok && im.boards != nil {
		source, board, canonical, ok = im.boards.Resolve(ctx, raw)
	}
	if !ok || registry[source] == nil || sources.ProviderKind(registry, source) != "ats" {
		return NativeDestination{}, fmt.Errorf("destination is not a recognized, crawlable ATS board")
	}
	if im.q != nil {
		cfg, err := im.q.GetIngestSchedule(ctx, source)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return NativeDestination{}, err
		}
		if err == nil && !cfg.Enabled {
			return NativeDestination{}, fmt.Errorf("native provider %s is disabled by its operator", source)
		}
	}
	known := false
	if catalog != nil {
		var err error
		_, known, err = catalog.Entry(ctx, source, board)
		if err != nil {
			return NativeDestination{}, err
		}
	}
	if !known {
		var err error
		employerWebsite, err = im.verifyEmployerBoard(ctx, row, source, board, employerWebsite)
		if err != nil {
			return NativeDestination{}, err
		}
	}
	c := destinationCatalog{stored: catalog, fallback: sources.CompanyEntry{Provider: source, Board: board, Company: row.Company, Region: atsboard.Region(canonical)}}
	e, _, err := c.Entry(ctx, source, board)
	if err != nil {
		return NativeDestination{}, err
	}
	verified := NativeDestination{Entry: e, EmployerWebsite: employerWebsite}
	j, found, err := linksource.ResolveOnBoard(ctx, registry, c, source, board, raw)
	if err != nil {
		return verified, err
	}
	if !found || j.Unreadable || j.Removed || strings.TrimSpace(j.Description) == "" {
		return verified, fmt.Errorf("native ATS did not return a readable live posting for the destination")
	}
	if normalize.CompanySlug(j.Company) != normalize.CompanySlug(row.Company) || !strings.EqualFold(strings.Join(strings.Fields(j.Title), " "), strings.Join(strings.Fields(row.Title), " ")) {
		return verified, fmt.Errorf("native posting's employer or title differs; refusing automatic identity transfer")
	}
	verified.Job = j
	return verified, nil
}
