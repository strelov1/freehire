# LinkedIn source adapter

Implement the requested adapter using the public HTML verified by the LinkedIn spike.
A board is a search keyword and its region is the search location; both are required.
Each bounded run reads up to ten recent-result pages, hydrating only unseen postings.
The employer comes from the posting, not the board label. Native numeric posting IDs
and canonical LinkedIn URLs identify postings; descriptions retain sanitized HTML.

## Approaches

Use the existing source registry and hydrating interface, with a separate paced
`cmd/liveness` pass. Search-result disappearance cannot establish closure because
results are capped. A complete-company listing cannot be claimed from this experiment.
Authenticated browser crawling would add credentials and infrastructure the public
HTML sample did not need, so it is outside this change.

## Lifecycle

Exclude LinkedIn from every unseen-job sweep and search's unseen timeout. Only an
explicit closed notice on the verified posting page supplies an expired verdict;
the existing liveness worker requires two separate confirmations before closing.
HTTP failures, login walls, incomplete pages, ID mismatches and unknown layouts are
uncertain. A primary apply control on a complete verified page confirms live.
Rediscovery reopens postings through the existing ingest pipeline.

## Bounds and limits

Pace all adapter requests across boards at one per two seconds. Limit one board to
ten listing pages, fail an unrecognized first listing, and preserve unreadable detail
markers. The liveness pass has the same pace, stops on refusal, and probes at most
300 stale postings per run. These are conservative initial budgets, not measured
sustained LinkedIn limits. Do not change production boards, schedules, or credentials.
An actual lifecycle transition over days was not observed by the spike.
