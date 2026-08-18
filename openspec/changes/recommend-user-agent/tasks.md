## 1. Publish the convention

- [x] 1.1 Add a short identification section to `web/static/openapi.yaml`, beside
      the rate-limit section it exists to serve, giving the format and stating
      plainly that nothing validates or requires it
- [x] 1.2 Add the same to `web/static/llms.txt`, in the bot-facing section that
      already tells agents to prefer the API over the pages
- [x] 1.3 Add a one-line pointer to `robots.txt`, keeping the body ASCII as the
      surrounding comments already are

## 2. Keep it a request

- [x] 2.1 Confirm no code path reads, validates or branches on `User-Agent` for
      public reads — the rate-limit refusal log may record it for diagnosis, which
      is observation, not enforcement
- [x] 2.2 `redocly lint` and the web lint stay clean; no Go change means no Go
      suite to run
