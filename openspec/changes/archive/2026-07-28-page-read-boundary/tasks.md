## 1. The boundary

- [x] 1.1 Add failing tests for a new `lib/tools/readable.ts`: `http` and `https` urls are readable; `chrome://`, `chrome-extension://`, `file://`, `about:` and an unparseable string are not. (The rule goes in a pure module rather than being asserted through a fake `PageBridge` — a test over the fake would assert the fake, and `page.ts` is chrome plumbing the repo does not test.)
- [x] 1.2 Write `readable.ts`, and call it from `activeTabPage.readPage` in `lib/tools/page.ts`: query the active tab and refuse before asking for a snapshot.

## 2. Attribution

- [x] 2.1 Add failing tests for a new `lib/assistant/pageRead.ts`: an ordinary result yields `host + path`; a result with a query string or fragment yields neither; a missing, malformed, or url-less result yields an empty string.
- [x] 2.2 Write `pageRead.ts` — pure over the tool call, no DOM.
- [x] 2.3 Show it in `ToolGroupList.svelte`, appended to the call's line, falling back to the plain label when it is empty.

## 3. Documentation

- [x] 3.1 Update `AGENTS.md`: `read_page` is bounded to `http(s)`, and the panel attributes each read. Note why the display logic is not in `tool-formatters.ts`.
- [x] 3.2 Update `docs/chrome-web-store.md`: the "Website content" declaration should say reads are bounded to ordinary web pages and shown in the transcript.

## 4. Verify

- [x] 4.1 `npm test`, `npm run check`, `npm run build`.
