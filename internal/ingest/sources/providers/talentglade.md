# Talentglade

Tenant sites use `<slug>.talentglade.com`. The public JSON endpoint on the same host is `/__careers?company_slug=<slug>&type=jobs`. Detail adds `type=job&job_id=<public_id>` instead. The public ID is the dashless, lower-case UUID and is used by the site's `/apply/<public_id>` route.

Verified against the Phare Consulting Group tenant on 2026-11-22. The listing returned five of five jobs. The detail endpoint returned descriptions for active IDs. An expired ID returned HTTP 200 with `closed_title` populated and a different job's ID and description. The adapter skips this replacement.
