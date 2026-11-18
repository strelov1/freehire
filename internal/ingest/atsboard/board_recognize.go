package atsboard

import (
	"net/url"
	"strings"
)

// Recognize parses a pasted job link into the company board it belongs to: the source
// (ATS provider), the board slug, and the canonical URL to store. ok=false when the host is
// not a supported ATS or the URL carries no board segment/label.
func Recognize(rawURL string) (source, board, canonical string, ok bool) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", "", "", false
	}
	host := hostname(u)
	if noBoardHosts[host] {
		return "", "", "", false
	}
	if src, board, canonical, ok := recognizeAPI(u, host); ok {
		return src, board, canonical, true
	}
	src, mode, apex, known := matchHost(host)
	if !known {
		return "", "", "", false
	}

	switch mode {
	case modeSubdomain, modeSubdomainChain, modeHost:
		// The vendor's own product host must not be read as a tenant, regardless of which of
		// the three modes matched: all three derive the board from host's leftmost DNS label
		// (bare, chained, or as the whole host), and a platform label like "app" or "help" sits
		// in that exact position for a subdomain tenant just as much as for a bare-host one —
		// e.g. app.recruitee.com and help.bamboohr.com are the vendor's own login/support hosts,
		// not a company named "app" or "help".
		if platformHost(host, apex) {
			return "", "", "", false
		}
		switch mode {
		case modeSubdomain:
			board = subdomainLabel(host, apex)
		case modeSubdomainChain:
			board = subdomainChain(host, apex)
		default:
			board = host // the whole careers host is the tenant identity
		}
		if board == "" {
			return "", "", "", false // bare apex, no tenant label
		}
		// The board IS the host, so the canonical URL is the bare host — collapsing a vacancy
		// URL and the board listing to one board.
		u.RawQuery, u.Fragment, u.Path = "", "", ""
		return src, board, u.String(), true

	case modeHostPath:
		// Workday: board = "<host>/<site>" where site is the first path segment (case-preserved,
		// as the ingest stores it). Workday's public URL may prefix the site with an xx-XX locale
		// the CXS API omits (host/en-US/<site>), so skip a leading locale — a locale-prefixed URL
		// and a bare one resolve to the same board. A URL carrying ONLY a locale (host/en-US, itself
		// a 404 landing) has no derivable site and is left unrecognized rather than taken as a false
		// "en-US" board. Canonical strips to scheme://host/site.
		site := workdaySite(u)
		// A Workday per-job URL can reach us with no site at all (host/job/... or
		// host/details/...), and those segments are the POSTING, never a career site. Taking one
		// as the site yields "<host>/job" — a board that does not exist but looks new, so the
		// contribution flow records and pays for it.
		if site == "" || site == "job" || site == "details" {
			return "", "", "", false // bare host, locale-only, or a per-job path with no site
		}
		u.RawQuery, u.Fragment = "", ""
		u.Path = "/" + site
		return src, host + "/" + site, u.String(), true

	case modeHostTenantBoard:
		// UKG: <host>/<tenant>/JobBoard/<guid>/… → board "<host>/<tenant>/<guid>". The literal
		// "JobBoard" segment between the two is what proves the URL names a board at all; without
		// it the path is some other part of the tenant's site (a login, an OpportunityDetail
		// shortlink) and there is no guid to take. Declining is the only safe reading — the
		// tenant alone is a board id the adapter rejects, and a rejected board still costs a
		// contribution row and a crawl slot.
		tenant, guid, ok := ukgTenantBoard(u)
		if !ok {
			return "", "", "", false
		}
		// Lower-cased: UKG answers either spelling (verified live against a real tenant, upper and
		// lower, both 200) and every board in the catalogue is stored lower-case, so keeping a
		// link's own casing would file a board we already crawl as a new one. This is the opposite
		// of Workday above, whose site segment is a human-chosen name the ingest stores verbatim.
		board = strings.ToLower(host + "/" + tenant + "/" + guid)
		u.RawQuery, u.Fragment = "", ""
		u.Path = "/" + tenant + "/JobBoard/" + guid
		return src, board, u.String(), true

	case modeHostCareers:
		// UKG Ready: <host>/ta/<tenant>.careers[?ShowJob=…] → board "<host>/<tenant>". Lower-cased
		// because tenant ids are case-insensitive at the platform and ukgready.yml holds them
		// lower-case, so keeping a link's own spelling would file a board we already crawl as new.
		tenant, ok := ukgReadyTenant(u)
		if !ok {
			return "", "", "", false
		}
		u.RawQuery, u.Fragment = "", ""
		u.Path = "/ta/" + tenant + ".careers"
		return src, host + "/" + tenant, u.String(), true

	case modeQuery:
		// Paycor: the board is a query parameter, and the canonical is the board's own listing —
		// so a posting and the career home collapse to one. Lower-cased by default for the same
		// reason as UKG Ready: the platform serves either spelling of the hex client id and
		// paycor.yml holds it lower-case.
		q, configured := queryBoards[apex]
		if !configured || q.boardPattern == nil {
			return "", "", "", false
		}
		value := u.Query().Get(q.param)
		if !q.keepCase {
			value = strings.ToLower(value)
		}
		// The pattern rejects an absent or empty parameter along with a malformed one, so a link
		// on the host that names no board is declined rather than turned into one.
		if !q.boardPattern.MatchString(value) {
			if q.altParam == "" || q.altPattern == nil {
				return "", "", "", false
			}
			m := q.altPattern.FindStringSubmatch(strings.ToLower(u.Query().Get(q.altParam)))
			if len(m) < 2 || !q.boardPattern.MatchString(m[1]) {
				return "", "", "", false
			}
			value = m[1]
		}
		board = value
		if q.tenantHost {
			if platformHost(host, apex) || subdomainLabel(host, apex) == "" {
				return "", "", "", false // vendor host, bare apex, or deeper than one tenant label
			}
			board = host + "/" + value
		}
		query := url.Values{q.param: {value}}
		for k, v := range q.listingQuery {
			query[k] = v
		}
		u.Fragment = ""
		u.Path = q.listingPath
		u.RawQuery = query.Encode()
		return src, board, u.String(), true

	default:
		return recognizeRest(u, host, src, mode, apex)
	}
}

// recognizeAPI resolves a URL on an ATS's own API host, where the board is the segment right
// after the entry's fixed path prefix. The canonical collapses to that prefix + board, so every
// query-parametered variant of the same API call maps to one board.
func recognizeAPI(u *url.URL, host string) (source, board, canonical string, ok bool) {
	for _, a := range apiBoards {
		if host != a.host {
			continue
		}
		// The prefix matches in any case: an embed spells the path however its author typed it,
		// and these hosts route case-blind.
		path := strings.Trim(u.Path, "/")
		if len(path) <= len(a.prefix) || !strings.EqualFold(path[:len(a.prefix)+1], a.prefix+"/") {
			return "", "", "", false
		}
		rest := path[len(a.prefix)+1:]
		if board, _, _ = strings.Cut(rest, "/"); board == "" {
			return "", "", "", false
		}
		u.RawQuery, u.Fragment = "", ""
		u.Path = "/" + a.prefix + "/" + board
		return a.source, board, u.String(), true
	}
	return "", "", "", false
}

// matchHost returns the ATS entry for a host. path/subdomain entries match the host exactly or
// as a subdomain of the entry host (the returned apex). A host entry keys on a domain LABEL
// (e.g. "zohorecruit") and matches any host containing ".<label>." — a tenant subdomain on any
// regional TLD (<tenant>.zohorecruit.eu/.com/.in); the bare apex ("zohorecruit.com") does not
// match, so it is never taken as a board.
func matchHost(host string) (source, mode, apex string, ok bool) {
	for _, a := range atsBoards {
		if a.mode == modeHost {
			// A bare vendor name matches under any TLD (<tenant>.factorial.<tld>); a dotted one is
			// a fixed apex (bizneo.cloud), so the vendor's other domains stay unmatched.
			if strings.Contains(a.host, ".") {
				if strings.HasSuffix(host, "."+a.host) {
					return a.source, a.mode, a.host, true
				}
				continue
			}
			if strings.Contains(host, "."+a.host+".") {
				return a.source, a.mode, a.host, true
			}
			continue
		}
		if host == a.host || strings.HasSuffix(host, "."+a.host) {
			return a.source, a.mode, a.host, true
		}
	}
	return "", "", "", false
}

// icimsBoardFromHost derives an iCIMS board from a *.icims.com host, mirroring the ingest
// adapter's own icimsVanity/icimsHost split: the classic two-label "careers-<slug>.icims.com"
// shape (the platform's own default hostname) folds to the bare slug, matching how icims.yml
// already stores most boards; any other *.icims.com host — including one that merely LOOKS like
// the classic shape but doesn't carry the "careers-" prefix (e.g. uscareers-lennox.icims.com,
// a client's own vanity choice) — is itself the board, kept whole. Returns "" for the bare apex
// or a host outside icims.com entirely.
func icimsBoardFromHost(host string) string {
	if !strings.HasSuffix(host, ".icims.com") {
		return ""
	}
	label := strings.TrimSuffix(host, ".icims.com")
	if slug, ok := strings.CutPrefix(label, "careers-"); ok && slug != "" && !strings.Contains(label, ".") {
		return slug
	}
	return host
}

// subdomainChain returns every DNS label of host under apex:
// "thefjx.global.huntflow.io","huntflow.io" → "thefjx.global"; "huntflow.io",… → "" (no tenant).
func subdomainChain(host, apex string) string {
	sub := strings.TrimSuffix(host, "."+apex)
	if sub == host {
		return ""
	}
	return sub
}

// subdomainLabel returns the leftmost DNS label of host under apex:
// "acme.recruitee.com","recruitee.com" → "acme"; "recruitee.com",… → "" (no tenant).
//
// A remainder with more than one label yields NO board, deliberately. These adapters crawl
// "<board>.<apex>", so a regional or nested host like "uk-ext.eu.csod.com" has no such form —
// taking "uk-ext" names a host that does not exist. That is the expensive direction: the
// contribution flow pays for a board it believes is new. Where a platform genuinely nests a
// tenant under an instance, the entry uses subdomainchain and keeps every label.
func subdomainLabel(host, apex string) string {
	sub := subdomainChain(host, apex)
	if strings.Contains(sub, ".") {
		return ""
	}
	return sub
}
