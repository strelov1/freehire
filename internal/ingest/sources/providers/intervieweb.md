# Intervieweb

The board is the full careers host. Read `/en/career` normally, then the site's
AJAX pager. Adding `X-Requested-With` to the first GET changes some tenants'
response into a shell; that is not a useful control for the normal adapter.

MSC Technology Italia's live listing includes job 83980, IT Network Operation
Center Specialist. Verified 2026-11-15: that detail page has no JobPosting JSON-LD,
while neighboring postings do. It publishes `#description__vacancy-title`,
`#description__body`, and a `span[title=Location]`, plus a share element whose
`data-a2a-url` contains the posting's identity. The HTML fallback requires the
share URL's ID to match and a nonempty title and body. It leaves unprovided dates
absent. Do not scrape the entire page: its application form and privacy policy
are much larger than the job description.
