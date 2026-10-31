# scrapling-fetch

The fetch service behind the ingest crawl's Scrapling tier (`internal/platform/scraplingfetch`,
`scraplingProviders` in `internal/ingest/sources`). It fetches pages for the sources a plain Go
client cannot reach — an IP the source refuses, a TLS fingerprint it refuses, a Cloudflare
challenge — and hands back exactly the bytes the adapter would have read.

Two modes, both from [Scrapling](https://github.com/D4Vinci/Scrapling):

- `http` — one request through curl_cffi with Chrome's TLS fingerprint. GET, POST, PUT, DELETE.
  ~0.2-2 s.
- `browser` — a navigation in a stealth Chromium session that solves Cloudflare challenges.
  GET only, and the caller's headers are not sent: the browser presents its own, which is the
  point of using it. The first page on a host costs the challenge (measured: bayt ~110 s); later pages on
  the same warm session ~5-9 s.

## API

```
POST /fetch   Authorization: Bearer $SCRAPLING_FETCH_TOKEN
  {"url": "...", "method": "GET", "headers": {...}, "body": "<base64>", "mode": "http"|"browser", "proxy": false}
  -> 200 {"status": 403, "url": "<final url>", "headers": {...}, "body": "<base64>"}   upstream answer, as data
  -> 400/401/413 {"error": "..."}                                                      nothing was fetched
  -> 502 {"error": "..."}                                                              the fetch itself failed
GET /health -> {"status": "ok"}
```

The upstream status is DATA: a 403 page comes back with HTTP 200 and `"status": 403`. Only a
fetch that could not complete is a 502. `proxy: true` egresses through `SOURCES_PROXY_URL`, which
only the service reads — no proxy credential crosses the wire. The caller's `User-Agent` and
transport headers are dropped (they would undo the impersonation), and the returned headers omit
`Content-Encoding`/`Content-Length`/`Transfer-Encoding`, because the body comes back decoded.

## Browser sessions

One warm session per `(host, proxy)`, reused across requests so a challenge is solved once per
crawl. A session serves up to `SCRAPLING_TABS_PER_BROWSER` pages at once, one tab each — much
cheaper than a browser per page — and a host needing more than that gets several sessions. At most `SCRAPLING_MAX_BROWSERS` live at once
(~1-1.7 GB each); past that an idle session of another host is closed, and if none is idle the
request waits — for at most 120 s, after which it fails with a 502 rather than queue on behalf
of a caller that has already given up. A session unused for `SCRAPLING_IDLE_SECONDS` is closed; a session whose page
raised takes no new pages and closes once its other tabs finish.

Memory does not leak through a session that never idles: a session in constant use (a crawl
running for hours) is retired after `SCRAPLING_SESSION_MAX_SECONDS` or
`SCRAPLING_SESSION_MAX_PAGES`, whichever comes first, and a page still running after 170 s is
cut off and its session retired, so one hung tab cannot hold a slot for ever. Behind all of it,
the container's memory cap and `Restart=always` catch anything the pool does not.

## Proxy rotation

`http` mode through the proxy goes through the ROTATING gateway: every request leaves from a new
exit IP, so a source's per-IP rate limit (Wellfound answered 429 to one shared IP) sees each
request from a different address. A browser session must not rotate — a Cloudflare clearance
belongs to the exit IP it was solved from — so each new proxied session takes the next STICKY
exit IP in turn and keeps it for its life. A page a source refuses with 403 or 429 is still
returned to the caller, but its session is retired, so the next opens on another IP. A browser session is also retired on 405, which is how InfoJobs refuses; over `http` a 405 is returned without a retry, since its callers re-send it under their own budgets.

With Webshare, the rotating gateway is `http://<user>-rotate:<pass>@p.webshare.io:80` and the
sticky template `http://<user>-{n}:<pass>@p.webshare.io:80`, one fixed IP per `n`.

## Configuration

| variable | default | |
|---|---|---|
| `SCRAPLING_FETCH_TOKEN` | — | required |
| `SCRAPLING_FETCH_ADDR` | `127.0.0.1:8098` | the crawlers run on the same host |
| `SCRAPLING_MAX_BROWSERS` | `3` | |
| `SCRAPLING_IDLE_SECONDS` | `600` | |
| `SCRAPLING_TABS_PER_BROWSER` | `4` | concurrent pages one browser session serves |
| `SCRAPLING_SESSION_MAX_SECONDS` | `1800` | a session this old is retired even while in use |
| `SCRAPLING_SESSION_MAX_PAGES` | `500` | a session is retired after this many pages |
| `SOURCES_PROXY_URL` | — | the single proxy `proxy: true` egresses through, when the two below are unset |
| `SCRAPLING_PROXY_ROTATING` | `SOURCES_PROXY_URL` | a gateway handing out a new exit IP per request — what `http` mode uses |
| `SCRAPLING_PROXY_STICKY` | `SOURCES_PROXY_URL` | a proxy URL template with `{n}`, one fixed exit IP per `n` — what browser sessions use |
| `SCRAPLING_PROXY_STICKY_COUNT` | — | how many `n` the template has (required with it) |

## Run

```
docker build -t freehire/scrapling-fetch services/scrapling-fetch
docker run --network host -e SCRAPLING_FETCH_TOKEN=... -e SOURCES_PROXY_URL=... freehire/scrapling-fetch
curl -s 127.0.0.1:8098/fetch -H "Authorization: Bearer $TOKEN" \
  -d '{"url":"https://www.bayt.com/en/uae/jobs/","mode":"browser"}' | jq '.status'
```

On host2 it runs under a systemd unit kept in `freehire-ops` (`provision/host2/`). `release.sh`
does not build or restart it: an image bump is a manual `docker build` and unit restart.

## Test

```
pip install -r requirements-test.txt
python -m pytest -q
```

The tests fake both fetchers and the session factory, so they need neither Scrapling nor a
browser. What they cannot cover — that a real page passes a real challenge — is checked live
against the deployed service.
