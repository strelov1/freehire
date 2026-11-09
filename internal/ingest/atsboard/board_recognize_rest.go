package atsboard

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"slices"
	"strings"
)

// recognizeRest is the second half of Recognize's switch on the extraction mode, split out only
// to keep each file small: it takes the modes Recognize does not handle itself and keeps the
// original fail-safe default.
func recognizeRest(u *url.URL, host, src, mode, apex string) (source, board, canonical string, ok bool) {
	switch mode {
	case modePathPattern:
		p, configured := pathPatternBoards[apex]
		if !configured || p.path == nil || p.listing == "" {
			return "", "", "", false
		}
		if p.exactHost && !strings.EqualFold(u.Hostname(), apex) {
			// u.Hostname(), not the www.-trimmed host (case-folded, as hostnames are
			// case-insensitive): the gate pins the entry to the exact host the platform
			// serves, and www.jobs.dover.com is NOT that host — Dover does not
			// serve boards there, so recognizing it would mint boards off a hostname the shell
			// cannot back. (The trim is fine for every other mode, whose apexes are the SURFACE
			// family — www.<board>.teamtailor.com is a real tenant host — but an exact-host entry
			// names one literal host.)
			return "", "", "", false
		}
		if p.exactHost {
			u.Host = apex // one board, one canonical: the link's host spelling is not identity
		}
		if m := p.path.FindStringSubmatch(u.Path); len(m) == 2 && m[1] != "" {
			board = m[1]
		} else if p.param != "" && p.paramPattern != nil {
			board = strings.ToLower(u.Query().Get(p.param))
			if !p.paramPattern.MatchString(board) {
				return "", "", "", false
			}
		} else {
			return "", "", "", false
		}
		if p.fold {
			board = strings.ToLower(board)
		}
		if p.reject != nil && p.reject.MatchString(board) {
			return "", "", "", false
		}
		u.RawQuery, u.Fragment = "", ""
		u.Path = fmt.Sprintf(p.listing, board)
		switch {
		case p.hostBoard:
			board = apex + "/" + board
		case p.tenantHost:
			board = host + "/" + board
		}
		return src, board, u.String(), true

	case modeQueryPair:
		// ADP Workforce Now, Taleo Business Edition: the board is two parameters, and the
		// canonical is the listing carrying just those two — so a posting link and a bare
		// career-site link collapse to one board.
		q, configured := queryPairBoards[apex]
		if !configured || q.firstPattern == nil || q.secondPattern == nil || q.foldFirst == nil {
			return "", "", "", false
		}
		// Each pattern rejects an absent or empty parameter along with a malformed one, so a link
		// on the host naming only one half is declined rather than turned into a truncated board.
		first := q.foldFirst(u.Query().Get(q.first))
		second := u.Query().Get(q.second)
		if !q.firstPattern.MatchString(first) || !q.secondPattern.MatchString(second) {
			return "", "", "", false
		}
		board = first + ":" + second
		query := url.Values{q.first: {first}, q.second: {second}}
		for k, v := range q.listingQuery {
			query[k] = v
		}
		u.Fragment = ""
		u.Path = q.listingPath
		u.RawQuery = query.Encode()
		return src, board, u.String(), true

	case modePathLocalePair:
		// Dayforce: <host>/<culture?>/<tenant>/<site>/… → board "<tenant>/<site>". The culture is
		// dropped, not kept: it selects which translations of a site's postings to read and a
		// posting keeps one id across them, which is why the ingest adapter folds it off too. The
		// canonical collapses to the culture-free site root (verified live: it answers 200 with and
		// without one). Lower-cased — tenant and site are case-insensitive at the platform and
		// dayforce.yml holds them lower-case.
		segs := segmentsAfterLocale(u)
		// The no-board check reads the segment the board would START at, not the URL's first —
		// the platform's own paths take a locale prefix too, and "/en-US/api/geo/…" would
		// otherwise walk past the guard and name the board "api/geo".
		if len(segs) < 2 || segs[0] == "" || segs[1] == "" || slices.Contains(noBoardFirstSegments[apex], segs[0]) {
			return "", "", "", false // bare host, locale-only, a tenant with no site, or machinery
		}
		board = strings.ToLower(segs[0] + "/" + segs[1])
		u.RawQuery, u.Fragment = "", ""
		u.Path = "/" + board
		return src, board, u.String(), true

	case modePathPair:
		// werecruit: <host>/<locale>/<tenant>/… → board "<locale>/<tenant>". Both segments are
		// required and neither is dropped — the locale is not optional here (see modePathPair's
		// doc comment), so this is a plain two-segment capture, not a locale-detection step.
		segs := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(segs) < 2 || segs[0] == "" || segs[1] == "" {
			return "", "", "", false // bare host or a locale with no tenant
		}
		if slices.Contains(noBoardTenantSegments[apex], strings.ToLower(segs[1])) {
			return "", "", "", false // the platform's own page in the tenant's place
		}
		board = segs[0] + "/" + segs[1]
		u.RawQuery, u.Fragment = "", ""
		u.Path = "/" + board
		return src, board, u.String(), true

	case modePathPortal:
		// SmartRecruiters' Apply button leads to a one-click form under the product's own
		// path, which names the employer instead of leading with it. It is matched first
		// because the generic rule below would read "oneclick-ui" as the board — a tenant
		// that does not exist, which the contribution flow would record and pay for.
		if m := smartrecruitersOneClick.FindStringSubmatch(u.Path); m != nil {
			u.RawQuery, u.Fragment = "", ""
			u.Path = "/" + m[1]
			return src, m[1], u.String(), true
		}
		// Otherwise the employer is the segment immediately before the posting, which is
		// the first segment on a bare URL and the second behind a portal segment. Only a
		// recognizable posting segment shifts the board along; any other shape falls through to
		// the first segment, so an unknown URL shape can't invent a board.
		board = segmentBeforePosting(u, smartrecruitersPosting)
		if board == "" {
			return "", "", "", false
		}
		u.RawQuery, u.Fragment = "", ""
		return src, board, u.String(), true

	case modeICIMS:
		board = icimsBoardFromHost(host)
		if board == "" {
			return "", "", "", false
		}
		u.RawQuery, u.Fragment, u.Path = "", "", ""
		return src, board, u.String(), true

	case modePathLocale:
		// Rippling: skip a leading xx-XX locale segment (ats.rippling.com/en-GB/<board>/…),
		// which the board API omits, and collapse the canonical to the board root so a
		// locale-prefixed vacancy, a bare vacancy, and the listing all map to one board.
		board = firstSegmentAfterLocale(u)
		if board == "" || slices.Contains(noBoardFirstSegments[apex], board) {
			return "", "", "", false
		}
		u.RawQuery, u.Fragment = "", ""
		u.Path = "/" + board
		return src, board, u.String(), true

	case modeHostSite:
		// Paradox: <host>/co/<site>/… → board "<host>/<site>". The bare apex is the vendor's
		// marketing site, never an instance, and a path not under "/co/" names no career site.
		segs := strings.Split(strings.Trim(u.Path, "/"), "/")
		if host == apex || len(segs) < 2 || !strings.EqualFold(segs[0], "co") || !paradoxSite.MatchString(segs[1]) {
			return "", "", "", false
		}
		site := strings.ToLower(segs[1])
		u.RawQuery, u.Fragment = "", ""
		u.Path = "/co/" + site + "/jobs"
		return src, host + "/" + site, u.String(), true

	case modeAllowedFirstSegment:
		// Dover: board = the segment right after an ALLOWED leading segment ("apply"/"jobs").
		// Any other leading segment — or a leading segment with nothing behind it — carries no
		// board and is declined, rather than being read as a tenant the way the deny-list path
		// mode would.
		allowed := allowedFirstSegments[apex]
		segs := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(segs) < 2 || segs[0] == "" || segs[1] == "" || !slices.Contains(allowed.segments, segs[0]) {
			return "", "", "", false
		}
		board = segs[1]
		u.RawQuery, u.Fragment = "", ""
		// The canonical collapses to the board's own listing page, so an apply link and the
		// listing itself map to one board — the same collapse every other mode makes.
		u.Path = "/" + allowed.listing + "/" + board
		return src, board, u.String(), true

	case modeTenantHost:
		if platformHost(host, apex) || subdomainLabel(host, apex) == "" {
			return "", "", "", false // vendor host, bare apex, or deeper than one tenant label
		}
		if seg := tenantHostPostingSegments[apex]; seg != "" &&
			!slices.Contains(strings.Split(strings.Trim(u.Path, "/"), "/"), seg) {
			return "", "", "", false
		}
		u.RawQuery, u.Fragment, u.Path = "", "", ""
		return src, host, u.String(), true

	case modeKeyedSegment:
		k, configured := keyedSegmentBoards[apex]
		if !configured || k.boardPattern == nil {
			return "", "", "", false
		}
		segs := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(segs) < 2 || segs[0] != k.key || !k.boardPattern.MatchString(segs[1]) {
			return "", "", "", false
		}
		board = segs[1]
		u.RawQuery, u.Fragment = "", ""
		u.Path = "/" + k.key + "/" + board
		return src, board, u.String(), true

	case modePathJob:
		// Harri: harri.com/<brand slug>/job/<id>-<title>. Anything else on the host — a career
		// page, a marketing page, an API host matched as a subdomain — is declined.
		segs := strings.Split(strings.Trim(u.Path, "/"), "/")
		if host != apex || len(segs) < 3 || segs[0] == "" || !strings.EqualFold(segs[1], "job") ||
			segs[2] == "" || segs[2][0] < '0' || segs[2][0] > '9' {
			return "", "", "", false
		}
		board = strings.ToLower(segs[0])
		u.RawQuery, u.Fragment = "", ""
		u.Path = "/" + board
		return src, board, u.String(), true
	case modeEncodedCareers:
		// IdealTraits: /<slug>/<base64 id>/careers → board = the decoded id. The decoded value
		// must be a plain decimal number: base64 decodes plenty of other paths into bytes, and
		// only the shape proves the segment is a business id. Canonical keeps the career-site
		// path as given, without query or fragment (the listing's ?page=N).
		segs := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(segs) != 3 || segs[0] == "" || segs[2] != "careers" {
			return "", "", "", false
		}
		decoded, err := base64.StdEncoding.DecodeString(segs[1])
		if err != nil || !businessID.Match(decoded) {
			return "", "", "", false
		}
		u.RawQuery, u.Fragment = "", ""
		return src, string(decoded), u.String(), true

	case modePath, modePathNumeric:
		// A tenant on its own subdomain of a path-based host is named by that label, whatever
		// its path says: reading the path would take a posting's "job" segment for the board.
		if pathTenantSubdomains[apex] && !platformHost(host, apex) {
			if label := subdomainLabel(host, apex); label != "" {
				u.RawQuery, u.Fragment, u.Path = "", "", ""
				return src, label, u.String(), true
			}
		}
		// The board is the first path segment that isn't platform machinery — declined
		// outright when the path leads with a segment that carries no board.
		if leadsWithNoBoard(u, noBoardFirstSegments[apex]) {
			return "", "", "", false
		}
		board = firstTenantSegment(u, reservedSegments[apex])
		if board == "" {
			return "", "", "", false
		}
		if mode == modePathNumeric && !allDigits(board) {
			return "", "", "", false
		}
		u.RawQuery = ""
		u.Fragment = ""
		u.Path = strings.TrimSuffix(strings.TrimSuffix(u.Path, "/"), "/apply")
		return src, board, u.String(), true

	default:
		// A mode with no branch of its own is a mode nobody has decided how to read, so
		// there is nothing to read it as. The two cases above used to be served by an
		// implicit tail after the switch, which silently made the first-path-segment rule
		// the fallback for every mode ADDED LATER too: a new platform that keeps its board
		// anywhere but the first path segment would have had whatever its URL leads with
		// taken as the tenant — a board that does not exist, which the contribution flow
		// records and pays for (see the package doc). Declining is the fail-safe reading:
		// the link is simply not recognized, which is what an unsupported host already gets.
		return "", "", "", false
	}
}
