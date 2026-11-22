# Catch Talents

The public careers host serves `/companies/<companyID>` and `/companies/<companyID>/jobs/<jobID>`. The public API exposes `GET /jobboard/companies/<companyID>/jobs?offset=0&limit=25` and `GET /jobboard/jobs/<jobID>` on `api.catch-talents.de`.

Checked against employers 259 and 2081 on 2026-11-22. The first employer returned 27 results over two pages with a 25-item limit. A limit of -1 returned HTTP 500. The employer listing identifies a posting by job ID and company ID, while the detail carries the description and open status.
