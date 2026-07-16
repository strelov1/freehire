## 1. Config

- [x] 1.1 Read `SOURCES_PROXY_URL` (form `http://user:pass@host:port`) via `url.Parse` in `sources.ApplyProxyEgress`; empty = disabled. Set-but-unparseable = error returned for fail-fast at worker startup. (Read where `sources.All` already reads source secrets like `REED_API_KEY`, not `internal/config` — matches existing style.)
- [x] 1.2 Opt-in per provider via a code-level `proxiedProviders` allowlist (marker), resolving the design open question toward a code set — no `SOURCES_PROXY_PROVIDERS` env (YAGNI; one provider today, adding the next is a one-line map entry).

## 2. Proxied transport

- [x] 2.1 `safehttp.NewClientWithProxy` / `NewTransportWithProxy(dialTimeout, *url.URL)`: SSRF-guarded transport with `Transport.Proxy = http.ProxyURL(u)`; nil proxy = exact current `NewClient`/`NewTransport` behavior. `sources.NewProxyClient(*url.URL)` builds the ingest `Client` over it.
- [x] 2.2 Test: nil proxy leaves `Transport.Proxy` unset and retains the guarded dialer; existing loopback-refusal tests still cover the direct path.

## 3. Per-provider wiring

- [x] 3.1 `sources.ApplyProxyEgress(registry)` builds the proxied `Client` when `SOURCES_PROXY_URL` is set and rewires each `proxiedProviders` entry onto it; called from `cmd/ingest` after `All`.
- [x] 3.2 No proxy configured → `ApplyProxyEgress` is a no-op; every adapter keeps the direct client.
- [x] 3.3 Test: proxied provider (`eightfold`) is rewired, a non-proxied provider (`greenhouse`) is unchanged; unset is a no-op.

## 4. Credential safety

- [x] 4.1 `SOURCES_PROXY_URL` is never logged raw; the invalid-value error passes through `redactProxy` (strips the password).
- [x] 4.2 Test: the invalid-URL error omits the proxy password; `redactProxy` strips creds for parseable and unparseable inputs.

## 5. Onboard eightfold + verify

- [x] 5.1 `eightfold` is the sole `proxiedProviders` entry; it uses the standard `JSONGetter` client, so the proxied `Client` serves it directly (not the fingerprint client).
- [x] 5.2 `go build ./... && go vet ./... && go test ./...` (77 pkgs) all pass; gofmt clean.
- [ ] 5.3 Deploy with `SOURCES_PROXY_URL` set on the ingest worker; trigger `freehire-ingest@eightfold`; confirm boards recover via the ingest monitor. **Pending: needs the proxy account provisioned in prod env.**

## 6. Docs

- [x] 6.1 Documented the env var + SSRF caveat (proxied path bypasses the target-IP guard; allowlist trusted hosts only) in `internal/sources/AGENTS.md`.
