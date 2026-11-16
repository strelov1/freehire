package linkimport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
)

// A board adapter may copy its catalogue label rather than read an employer from
// the posting. For a new board, verify the association independently through the
// employer's published website before supplying that label to the adapter.
func (im *Importer) verifyEmployerBoard(ctx context.Context, origin db.Job, provider, board, website string) (string, error) {
	if website == "" && im.q != nil {
		slug := origin.CompanySlug
		if slug == "" {
			slug = normalize.CompanySlug(origin.Company)
		}
		company, err := im.q.GetCompany(ctx, slug)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
		var info struct {
			Website string `json:"website"`
		}
		if err == nil && json.Unmarshal(company.CompanyInfo, &info) == nil {
			website = info.Website
		}
	}
	if website == "" && origin.Source == "linkedin" {
		if reader, ok := im.ingest["linkedin"].(interface {
			Posting(context.Context, string) (sources.Job, error)
			CompanyWebsite(context.Context, string) (string, error)
		}); ok {
			posting, err := reader.Posting(ctx, origin.URL)
			if err == nil && normalize.CompanySlug(posting.Company) == normalize.CompanySlug(origin.Company) {
				website = posting.CompanyWebsite
				if website == "" && posting.CompanyProfileURL != "" {
					website, _ = reader.CompanyWebsite(ctx, posting.CompanyProfileURL)
				}
			}
		}
	}
	resolver, ok := im.boards.(interface {
		ResolveCompany(context.Context, string) (string, string, string, bool)
	})
	if website != "" && ok {
		source, tenant, _, found := resolver.ResolveCompany(ctx, website)
		if found && source == provider && strings.EqualFold(tenant, board) {
			return website, nil
		}
	}
	return "", fmt.Errorf("native board's employer is unverified; its published company website must identify this ATS board")
}
