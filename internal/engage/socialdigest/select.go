package socialdigest

// Select applies the editorial rules to one day's candidates and returns the list to publish,
// most-viewed first. It is pure so the rules are cheap to test.
//
// candidates arrive ordered by PageUniques descending (see TopPageViewedJobsForDay) and already
// filtered to open, non-duplicate, non-private postings whose ATS still lists them. Left to
// decide here:
//
//   - MinPageUniques: a posting below the floor is noise, not popular.
//   - quarantined: a posting published within QuarantineDays must not return.
//   - MaxPerCompany: no employer takes more than its share of the list.
//   - Size: the list is at most this long.
//
// Fewer than Size, or none, is normal: an empty result is a quiet day and publishes nothing.
//
// The cap applies before truncation (one pass that stops at Size, not a filter then a slice),
// so a company's extra posting yields its place to someone else's and the list still fills.
func Select(candidates []Posting, quarantined map[int64]bool) []Posting {
	selected := make([]Posting, 0, Size)
	perCompany := make(map[string]int)

	for _, p := range candidates {
		if len(selected) == Size {
			break
		}
		if p.PageUniques < MinPageUniques {
			// continue, not break: correctness must not depend on the input ordering.
			continue
		}
		// Before the cap, so a quarantined posting does not spend its company's place.
		if quarantined[p.JobID] {
			continue
		}
		if perCompany[p.CompanySlug] >= MaxPerCompany {
			continue
		}
		perCompany[p.CompanySlug]++
		selected = append(selected, p)
	}
	return selected
}
