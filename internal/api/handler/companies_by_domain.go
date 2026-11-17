package handler

import (
	"context"
	"regexp"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
)

// maxCompanyDomains caps one by-domain lookup.
const maxCompanyDomains = 100

// The steps mirror public.company_site_host (migration 0283), in the same order.
var (
	siteSchemeRe = regexp.MustCompile(`^[a-z][a-z0-9+.-]*://`)
	sitePathRe   = regexp.MustCompile(`[/?#].*$`)
	siteUserRe   = regexp.MustCompile(`^.*@`)
	sitePortRe   = regexp.MustCompile(`:[0-9]*$`)
	siteDotsRe   = regexp.MustCompile(`[.]+$`)
)

// normalizeSiteHost reduces a bare domain or URL to its lowercase host without scheme, userinfo, port, path,
// trailing dot or leading www. "" when nothing is left. Keep in agreement with public.company_site_host.
func normalizeSiteHost(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = siteSchemeRe.ReplaceAllString(s, "")
	s = sitePathRe.ReplaceAllString(s, "")
	s = siteUserRe.ReplaceAllString(s, "")
	s = sitePortRe.ReplaceAllString(s, "")
	s = siteDotsRe.ReplaceAllString(s, "")
	return strings.TrimPrefix(s, "www.")
}

// CompaniesByDomain resolves company web domains to companies, for Data-API customers who hold a domain list.
func (h *companiesHandlers) CompaniesByDomain(ctx context.Context, _ apigen.CompaniesByDomainParams) (*apigen.CompanyByDomainPage, error) {
	var hosts []string
	seen := map[string]bool{}
	for _, raw := range facetValues(contractQuery(ctx), "domain") {
		if host := normalizeSiteHost(raw); host != "" && !seen[host] {
			seen[host] = true
			hosts = append(hosts, host)
		}
	}
	if len(hosts) == 0 {
		return nil, fiber.NewError(fiber.StatusBadRequest, "domain is required")
	}
	if len(hosts) > maxCompanyDomains {
		return nil, fiber.NewError(fiber.StatusBadRequest, "at most 100 domains per request")
	}
	rows, err := h.queries.ListCompaniesBySiteHost(ctx, hosts)
	if err != nil {
		return nil, err
	}
	matched := make(map[string]bool, len(rows))
	data := make([]apigen.CompanyByDomain, len(rows))
	for i, r := range rows {
		matched[r.Domain] = true
		data[i] = apigen.CompanyByDomain{Domain: r.Domain, Slug: r.Slug, Name: r.Name, JobCount: int64(r.JobCount), Website: r.Website}
	}
	unmatched := []string{}
	for _, host := range hosts {
		if !matched[host] {
			unmatched = append(unmatched, host)
		}
	}
	return &apigen.CompanyByDomainPage{Data: data, Meta: apigen.CompanyByDomainMeta{Unmatched: unmatched}}, nil
}
