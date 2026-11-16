# Clinch detail fixtures

Public HTML captured on 2026-11-15, reduced to the original `.job-title`,
`.job-component-list-location` and `.job-description` elements. Removed page chrome,
JavaScript, tracking and unrelated content; BeautifulSoup serialized the retained DOM.
Citadel/Moreton Bay retain repeated `.job-location` spans; Fire retains its explicit
`.job-component-location-group`. These verify theme variants and location deduplication.

- Armaguard: `https://armaguard-careers.careerpages.rec-marketing.dc2.pageuppeople.com/jobs/cash-processor-bendigo-vic-australia`
- Mader: `https://careers.madergroup.com.au/jobs/automotive-electricians-southwest-midwest-and-metro-western-australia-wa-australia`

These reproduce a false location boundary (`Cash` / `Processor Bendigo Vic`) and
loss of title punctuation/state capitalization. Metadata comes from explicit posting
components, never from splitting an HTML title on punctuation.

- Citadel (743): `https://jobs.citadel.edu/jobs/campus-services-officer-charleston-south-carolina-united-states`
- Moreton Bay (586): `https://careers.moretonbay.qld.gov.au/jobs/parks-labourer-expression-of-interest-eoi-grounds-maintenance-natural-areas-trees-city-of-moreton-bay-qld-australia`
- Fire (1138): `https://careers.fire.nsw.gov.au/jobs/on-call-firefighter-rn1-279-dorrigo-port-macquarie-nsw-australia-8d9050f0-73bd-4ba0-b320-6dafe43aee2c`
