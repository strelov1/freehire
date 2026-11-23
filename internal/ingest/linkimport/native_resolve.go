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

var ErrNativeATSUnrecognized = errors.New("destination is not a recognized, crawlable ATS board")

type NativeDestination struct {
	Entry           sources.CompanyEntry
	Job             sources.Job
	EmployerWebsite string
}

type destinationCatalog struct {
	stored   linksource.BoardCatalog
	fallback sources.CompanyEntry
}

func (c destinationCatalog) storedEntry(ctx context.Context, source, board string) (sources.CompanyEntry, bool, error) {
	if c.stored == nil {
		return sources.CompanyEntry{}, false, nil
	}
	if source == "lever" || source == "mokahr" {
		if regional, ok := c.stored.(interface {
			EntryForRegion(context.Context, string, string, string) (sources.CompanyEntry, bool, error)
		}); ok {
			entry, found, err := regional.EntryForRegion(ctx, source, board, c.fallback.Region)
			if source == "lever" && c.fallback.Region == "" && !found && err == nil {
				return regional.EntryForRegion(ctx, source, board, "us")
			}
			return entry, found, err
		}
	}
	e, found, err := c.stored.Entry(ctx, source, board)
	switch source {
	case "lever":
		found = found && (e.Region == "eu") == (c.fallback.Region == "eu")
	case "mokahr":
		found = found && e.Region == c.fallback.Region
	}
	return e, found, err
}

func (c destinationCatalog) Entry(ctx context.Context, source, board string) (sources.CompanyEntry, bool, error) {
	e, found, err := c.storedEntry(ctx, source, board)
	if err != nil || found {
		return e, found, err
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
		return NativeDestination{}, ErrNativeATSUnrecognized
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
	c := destinationCatalog{stored: catalog, fallback: sources.CompanyEntry{Provider: source, Board: board, Company: row.Company, Region: atsboard.Region(canonical)}}
	stored, known, err := c.storedEntry(ctx, source, board)
	if err != nil {
		return NativeDestination{}, err
	}
	// The catalogue may hold this board under another spelling of the same identity (a
	// Njoyn link on the generic clients host); the posting belongs under the stored one.
	if known {
		board = stored.Board
	}
	if !known {
		employerWebsite, err = im.verifyEmployerBoard(ctx, row, source, board, c.fallback.Region, employerWebsite)
		if err != nil {
			return NativeDestination{}, err
		}
	}

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
