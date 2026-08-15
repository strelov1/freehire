# Cloudflare for freehire.me

Puts a shared cache in front of the origin. The site is one Hetzner box in
Helsinki that also runs the ingest, and most of its traffic is anonymous reads of
pages that exist to be found in search — the same HTML, rendered again per
request, competing with the ingest for the buffer cache.

**Not applied yet.** The zone does not exist in the Cloudflare account, and the
domain's nameservers still point at Namecheap. Steps 1 and 2 below are manual.

## What it does

| | |
|---|---|
| DNS | `freehire.me`, `www`, `logo` → the origin, all proxied |
| TLS | `strict` — the origin already serves a valid public certificate |
| Transport | HTTP/3, 0-RTT, brotli |
| Cache | HTML held at the edge **on the origin's terms** |
| Bypass | `/api/`, `/my/`, `/auth/`, `/moderation`, `/cv/`, anything not GET |

The cache policy itself lives in the application, not here — see
`web/src/lib/httpCache.ts`. Anonymous public HTML is sent as
`public, max-age=0, s-maxage=300, stale-while-revalidate=86400`; anything tied to
a session is `private, no-store`. These rules only make HTML *eligible* for the
edge and then respect those headers, so there is one place that decides, and a
mistake in this file cannot leak a signed-in page.

Two independent guards keep sessions out of the shared cache: the origin's
`private, no-store`, and a cache key that includes the presence of `hire_token`.

### No bot blocking, on purpose

The sibling `telagon` config blocks scrapers by User-Agent (`python`, `curl`,
`Go-http-client`, `httpx`, …). That must not be copied here: freehire publishes
an HTTP API, a CLI and a ChatGPT action, so those clients are the intended
audience. `security_level` stays at `essentially_off` for the same reason — a
challenge page served to a crawler is a page that does not get indexed.

## Applying it

1. **Add the zone.** Cloudflare dashboard → Add a site → `freehire.me`, Free plan.
   Cloudflare will show two assigned nameservers.

2. **Repoint the nameservers at Namecheap** (the registrar today — the domain
   currently answers from `dns1/dns2.registrar-servers.com`). Domain List →
   Manage → Nameservers → Custom DNS → the two Cloudflare ones. Propagation is
   usually minutes, and the zone flips to Active on its own.

   Nothing below works until this is done: an inactive zone proxies nothing.

3. **Token.** Create one at [API Tokens](https://dash.cloudflare.com/profile/api-tokens)
   with `Zone Settings:Edit`, `DNS:Edit` and `Cache Rules:Edit`, scoped to this
   zone. Then:

   ```bash
   cd infra/cloudflare
   cp terraform.tfvars.example terraform.tfvars   # gitignored; paste the token
   terraform init
   terraform plan      # read this before applying
   terraform apply
   ```

## Checking it afterwards

```bash
# Served from the edge? (expect cf-cache-status, HIT on the second call)
curl -sI https://freehire.me/collections/python | grep -i 'cf-cache-status\|cache-control'

# A session must never be cached — expect DYNAMIC/BYPASS and private, no-store
curl -sI https://freehire.me/ -H 'Cookie: hire_token=whatever' | grep -i 'cf-cache-status\|cache-control'

# The API must not be cached
curl -sI 'https://freehire.me/api/v1/jobs/search?limit=1' | grep -i 'cf-cache-status'
```

The second command is the one that matters. If a request carrying `hire_token`
ever comes back `cf-cache-status: HIT`, stop and fix it before anything else.
