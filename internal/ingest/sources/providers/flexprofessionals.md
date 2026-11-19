**FlexProfessionals traps** (`flexprofessionals`; live-verified 2026-11-18 from a residential IP, not yet from the crawl host):

- **No feed, no ld+json: the REST type is the listing.** The site is WordPress with the Matador Jobs plugin; `/wp-json/wp/v2/matador-job-listings?per_page=100` returns every live posting (~33; `X-WP-Total` says how many) with the full body in `content.rendered`. `/feed/` is blog posts only and the detail pages carry only Yoast WebPage ld+json, no JobPosting.
- **Facets live in an HTML list at the top of `content.rendered`** (`matador-job-meta`: Category, Location, Work Arrangement, Type). The adapter reads Location / Work Arrangement (Remote, Hybrid, On-Site) / Type (Full-time/Flex, Part-time, Project-FT) from it and cuts the list off the description.
- **The company is always "FlexProfessionals".** It is a staffing firm and its clients are anonymous ("IT government contractor in Reston"), so there is no better employer name to take. Locations are US city, state with stray spaces ("Washington , District Of Columbia") and one posting has none.
- **Most roles are not tech** (accounting, HR, legal, admin); the pipeline filters downstream.
- **One page of 100.** Raise to paging on `X-WP-TotalPages` if the firm ever lists more.
