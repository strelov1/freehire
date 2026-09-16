## Context

See `proposal.md` - Why for the motivation and the live-form spike (Greenhouse, Lever) that
established no script-only path reaches `input.files`. Two further data points from
production open-source extensions targeting the same problem, gathered before writing this
design, both corroborate rather than complicate the approach:

- **Auto-Apply-Helper** (Greenhouse-focused, MIT, `extension/content/autofill.js`) attempts
  exactly the naive `inputElement.files = dataTransfer.files` assignment and ships its own
  "Upload may have failed - input.files is empty after setting" log line for when it silently
  doesn't take — it carries no `debugger` permission and no fallback.
- **jobright-automator** already spends the `debugger` permission in this problem space, for
  a neighboring reason: it drives the Apply *click* through CDP specifically because "a
  synthetic click would be stopped by Chrome's popup blocker" — the same "script-dispatched
  events aren't trusted" wall, one control over.

The extension already has the pieces this change composes: frame-aware form reading
(`lib/form.ts`, `extractUploads`), a job's public slug from the page (`lib/freehire.ts`,
`freehireSlugFromUrl`), and a tailored-CV list endpoint carrying `job_slug`
(`GET /me/cvs` → `cvTailoredResponse`) plus a PDF render endpoint
(`GET /me/cvs/:id/pdf`). Nothing on the hire side needs to change.

## Goals / Non-Goals

**Goals:**
- Place an already-tailored CV's rendered PDF into a detected file upload field, triggered
  by an explicit click, with the debugger attached for the shortest window that achieves it.
- Fail loudly and cleanly: a placement that doesn't succeed reports why, and never leaves
  `chrome.debugger` attached.

**Non-Goals:**
- Generating or choosing *which* tailored CV to use — this reads whatever `cv-tailoring`
  already produced for the job in view; it does not create one.
- Filling any other control on the form — that remains the existing deterministic/agent
  autofill's job (see the third added requirement in the spec delta).
- A cross-origin embedded iframe's upload field (e.g. a company site embedding
  `boards.greenhouse.io` behind a cross-origin `<iframe>`) reachable by a *different*
  render process than the top frame. See Risks.

## Decisions

**CDP addressing: resolve the input via `Runtime.evaluate` → `objectId`, not `DOM` node IDs.**
The content script already holds a live reference to the exact `<input type="file">`
element (`extractUploads` found it to decide the panel shows the action at all). Rather than
walking `DOM.getDocument`/`DOM.querySelector` from the debugger session to re-find it, the
attach step runs `Runtime.evaluate` scoped to that field's own frame with an expression that
re-locates the same element (by the same indexed selector `form.ts` already uses for
`fill_simple`'s `frame:form` addressing) and returns its `objectId`, then calls
`DOM.setFileInputFiles({objectId, files: [path]})`. This reuses the frame/form addressing the
codebase already trusts for `fill_simple` instead of introducing a second way to name "which
control."

**The file reaches disk through `chrome.downloads`, not an in-memory hand-off.**
`DOM.setFileInputFiles` takes filesystem paths, not bytes — CDP has no call that accepts a
`Blob`/`ArrayBuffer` for this. The flow is: fetch the PDF from
`GET /me/cvs/:id/pdf` → base64 it into a `data:` URL → `chrome.downloads.download` (into a
`freehire-tailored-cv/` subfolder, `conflictAction: 'uniquify'`) → poll
`chrome.downloads.onChanged` for `state.current === 'complete'` → `chrome.downloads.search`
for the resulting absolute `filename` → pass that path to CDP. The file is left in Downloads
rather than cleaned up afterward: it is a legitimate copy of the candidate's own tailored CV,
the same artifact the manual flow already has them download by hand, so leaving it is more
transparent than silently deleting a file Chrome just told the OS it saved.

**Attach → call → detach, wrapped so detach always runs.** `chrome.debugger.attach` opens the
visible banner; the call sequence (`Runtime.evaluate`, `DOM.setFileInputFiles`) is wrapped in
a try/finally that calls `chrome.debugger.detach` on every exit path, matching the spec's
"failed attach still leaves the debugger detached" requirement. `chrome.debugger.attach` also
fails outright if the tab is already open in real DevTools (Chrome allows one debugger client
per tab) — that failure is caught before any banner appears and reported as a plain "close
DevTools on this tab and try again," rather than a generic error.

**Alternatives considered:**
- *DataTransfer + synthetic `drop` dispatch, no debugger.* Rejected — this is what both the
  live-form spike and Auto-Apply-Helper's own failure log rule out for the ATS forms this
  extension actually meets.
- *Route the file through hire's existing `POST /me/autofill/run` agent path instead of a
  dedicated extension action.* Rejected for this change: that endpoint drives the
  browser-tool wire for an agent turn, and file placement has nothing for an agent to decide
  — it is one deterministic action gated on "does a tailored CV exist for this job," not a
  reasoning step. Folding it in would also mean the `debugger`/`downloads` permissions get
  requested on every agent-driven fill rather than only when the action is actually used.

## Risks / Trade-offs

- **[Embedded ATS iframe, cross-origin or not]** → A company site can embed Greenhouse behind
  an `<iframe>` (the "site-with-iframe" variant `extension/AGENTS.md` already names).
  `Runtime.evaluate`, unscoped, runs in the top frame's own default execution context and
  cannot reach a node in any other frame — this holds regardless of whether that frame
  happens to share an origin with the top one; nothing in this change distinguishes the two.
  Mitigation: `pickAttachableUpload` (`lib/tools/attachCv.ts`) considers only uploads reported
  in the top frame, and reports `unreachable` (surfaced as "can't reach this embedded form
  yet") when every upload the page offers sits in another one — never a silent no-op or a
  wrong-frame write. The panel's own visibility gate for the action (`App.svelte`'s
  `hasUploadField`) applies the same top-frame-only filter, so the action does not even
  appear for a page whose only upload is embedded.
- **[More than one upload field on the page]** → `Upload`/`FramedUpload` carry no label, so
  a form offering both a résumé and a cover-letter upload cannot be told apart by this
  action — guessing the first one risks writing the CV into the wrong field. Mitigation:
  `pickAttachableUpload` reports `ambiguous` (with the count) whenever more than one
  top-frame upload exists, and the caller refuses with a clear message rather than picking
  one; widening `Upload` to carry a label, so this could disambiguate the way `fillByLabel`
  does, is a larger change than this one and is left for if it turns out to matter in
  practice.
- **[Chrome's "ask where to save each file" setting]** → If the user has that setting on,
  `chrome.downloads.download` surfaces a native Save dialog instead of completing silently,
  stalling the flow on an OS-level prompt this code cannot see. Mitigation: bound the wait
  with a timeout and report "check your Downloads prompt" rather than hanging indefinitely.
- **[Chrome Web Store review]** → `debugger` is a sensitive permission and review may ask for
  justification or a narrower alternative. Mitigation: the permission is requested (declared
  in the manifest) unconditionally at install like any MV3 permission, but the banner and the
  actual attach only ever happen inside this one user-initiated action — nothing attaches the
  debugger on page load or in the background, which is the usual review concern.

## Migration Plan

No data migration. Ships as a new manifest permission pair (`debugger`, `downloads`) plus a
new panel action; existing installs pick it up on the extension's normal update channel. No
feature flag — the action is inert (never appears) for any caller with no tailored CV for the
job in view, which is the default state until they use `cv-tailoring` at least once.
