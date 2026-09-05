package socialdigest

// Select applies the editorial rules to one day's candidates and returns the list to
// publish, most-viewed first. It is a pure function over a slice so that the rules
// most likely to be argued about are the ones cheapest to test and change.
//
// candidates arrive already ordered by PageUniques descending (see
// TopPageViewedJobsForDay) and already filtered to open, non-duplicate, non-private
// postings whose ATS still lists them. What is left to decide here is editorial:
//
//   - MinPageUniques — a posting below the floor is not "popular", it is noise.
//   - quarantined — a posting published within QuarantineDays must not return, or a
//     posting that stays popular for a week leads the list every day for a week.
//   - MaxPerCompany — no employer takes more than its share of the list.
//   - Size — the list is at most this long.
//
// Returning fewer than Size is normal and returning none is fine: an empty result
// means the day was quiet, which the caller publishes as nothing at all.
//
// The cap applies BEFORE the list is truncated, which is why this is one pass that
// stops at Size rather than a filter followed by a slice. The two readings differ on
// a normal day: capping first means a company's third posting yields its place to
// someone else's and the list comes out full, while truncating first would leave a
// hole nothing refills and publish a short list. "Show me ten things" is the promise
// worth keeping to a reader; "show me the top ten minus repeats" is a promise only
// the person who wrote the rule can tell was kept.
func Select(candidates []Posting, quarantined map[int64]bool) []Posting {
	selected := make([]Posting, 0, Size)
	perCompany := make(map[string]int)

	for _, p := range candidates {
		if len(selected) == Size {
			break
		}
		if p.PageUniques < MinPageUniques {
			// The input is ordered by views descending, so nothing after this clears
			// the floor either — but breaking here would couple this function to that
			// ordering for correctness rather than only for its result's order.
			continue
		}
		// Checked before the cap is counted, so a quarantined posting does not spend
		// its company's place: the company still gets MaxPerCompany from what is left.
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
