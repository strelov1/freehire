# The LinkedIn XML job feed as a source

This project does not read the XML feed an ATS generates for LinkedIn. It reads each platform's
public jobs API or public feed instead.

## Why this is out of scope

- The feed is not public. An ATS generates it for LinkedIn and hands LinkedIn a private URL or token
  that belongs to the employer. None of the ATS platforms on LinkedIn's supported list that we
  already crawl exposes it at an address we can read.
- Where a public machine-readable source exists we already use it (Personio XML, Teamtailor RSS,
  Homerun, JobScore, Breezy and BambooHR JSON, Softgarden's `jobs.feed.json`), and for the large
  platforms the public jobs API carries more than the feed would.

Revisit if a platform publishes that feed at a public address, which would make it one more
adapter like the ones above.

## Prior requests

- #3990: "Specification of XML for ATS->Linkedin" (its ATS list did lead to moving some adapters
  onto public feeds)
