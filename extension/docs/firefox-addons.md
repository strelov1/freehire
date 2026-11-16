# Publishing to addons.mozilla.org

The Firefox build comes from the same source as the Chrome one (see
[../AGENTS.md](../AGENTS.md), "Firefox"). The listing copy, privacy text and
permission reasons are the same as in [chrome-web-store.md](chrome-web-store.md),
minus `sidePanel` and `debugger`, which the Firefox manifest does not ask for.

## First listing (once, by hand)

The pipeline can only add versions to an add-on that already exists, so the
first one is created in the browser:

1. Get the two files — download the `extension` artifact from the `build`
   workflow (or the latest `extension-v*` GitHub Release), or run
   `cd extension && pnpm run zip:firefox`:
   - `freehire-extension-<version>-firefox.zip` — the add-on;
   - `freehire-extension-<version>-sources.zip` — the source code.
2. [Developer Hub](https://addons.mozilla.org/developers/) → **Submit a New
   Add-on** → **On this site** (listed) → upload the `-firefox.zip`.
3. When asked whether the code is minified or bundled, say **yes** and upload the
   `-sources.zip`. Build instructions for the reviewer:

   > Node 24 and pnpm. From the archive root:
   > `cd design-system && pnpm install --frozen-lockfile && cd ../extension && pnpm install --frozen-lockfile && pnpm run build:firefox`.
   > The output is `extension/.output/firefox-mv2`.

4. Fill the listing from `chrome-web-store.md` and submit. Review is manual.

The add-on id is fixed in `wxt.config.ts` (`extension@freehire.me`), so sign-in
already works for the listed build — its redirect hash is on hire's allowlist.

## Every later version (automatic)

Add two repo secrets from Developer Hub → **Manage API Keys**:
`AMO_JWT_ISSUER` and `AMO_JWT_SECRET`. From then on, a push to `main` that bumps
`extension/package.json`'s `version` publishes a GitHub Release and submits the
same build to addons.mozilla.org (`.github/workflows/extension.yml`). Without the
secrets that step is skipped.
