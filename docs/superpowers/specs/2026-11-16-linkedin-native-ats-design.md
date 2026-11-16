# LinkedIn discovery with native ATS ownership

## Intent

The user corrected the previous application URL feature: an external Apply destination must become the posting's native source and recurring crawl owner. LinkedIn is discovery, and remains a public source only for confirmed LinkedIn-native applications (Easy Apply).

## Approach

Reuse ATS recognition, the board catalogue, native source adapters, and the existing ingest/sweep mechanism. A destination override alone does not satisfy this requirement. Do not invent an ATS from the employer name or label an unresolved application method as Easy Apply.

For the verified Gist destination, add HRsmart as a real provider, recognize its full tenant host, onboard the board through boardcatalog, and transfer the existing posting to the exact native identity. Preserve the job id, public slug and application history when no native identity exists; if a native posting already exists, reconcile without deleting job or application history. Clear the old application override during transfer. Native crawling supplies subsequent content and liveness.

## Discovery boundary

Classify the primary LinkedIn application control as native, external or unknown. External targets go through native ATS resolution; an unresolved external target is discovery work, not a public LinkedIn vacancy. Public LinkedIn guest HTML for Gist exposes an offsite marker but hides the URL; reuse the public company profile Organization.sameAs and follow observed careers links to discover its ATS board. Queue it for the ordinary ATS scheduler. No service session is required for this route. Never guess a host or publish an unresolved external discovery as LinkedIn-native.

## Acceptance

Gist source is hrsmart, native external identity is the full tenant host plus posting 3735, and its URL is the ATS posting. The board is scheduled through normal ingest. Recrawling updates the same job, and confirmed absence follows ATS sweep policy. Existing Freehire URL and history survive. Confirmed Easy Apply stays LinkedIn; external and unknown methods cannot be silently represented as native LinkedIn applications.
