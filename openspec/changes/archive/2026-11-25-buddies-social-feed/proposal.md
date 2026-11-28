## Why

The buddy feed is a column of identical cards, one per ledger event, that never link to the
posting they are about. The approved social redesign (see `buddies-social-space`) reads it
as posts: an interview or an offer shows the job as a link card, a member's applications on
one day fold into one post, and the feed is split by day.

## What Changes

- **API, additive:** each feed entry gains `job_slug` and `job_location` from the posting the
  event names (both null when there is none or it is gone).
- The feed reads as **posts under day headings**: an interview, an offer or a lone
  application is one post with the posting as a link card (to the job; to the company when
  the posting is gone); two or more applications by one member on one day are one post with
  the companies' logos fanned out over a list of the postings, each a link.
- Company logos in the space are drawn round on white (`BuddyCompanyLogo`).
- The feed page gets a right rail of similar members (`BuddySimilarRail`, shared with the
  member profile). Repeated Message buttons in lists use the outline style.
- A member's activity rows link the job title to the posting.

## Capabilities

### Modified Capabilities
- `buddies-social-space`: the feed's posts and job links.

## Impact

- **Go:** `ListBuddyFeed` selects `j.public_slug`, `j.location`; `buddy.FeedEntry`, the feed response.
- **Web:** `buddyFeed.ts` (grouping), `BuddyPost`, `BuddyJobCard`, `BuddyCompanyLogo`,
  `BuddySimilarRail`; `BuddyFeedEntry` keeps only its row form. No migration.
