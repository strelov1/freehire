---
name: verify-extension
description: Install the freehire browser extension built from this checkout into the owner's signed-in Chrome on macOS and drive it like a user through the cua_* computer-use tools, up to (never including) the final submit of an application. Use to prove an extension change on a real site such as LinkedIn Easy Apply, before calling it done, or when asked to install the latest extension build into Chrome.
---

# verify-extension

Proves an extension change where it runs: the build from THIS checkout, loaded unpacked
into the owner's real Chrome profile (signed in to LinkedIn and freehire.me), driven through
the side panel and the employer's page. Vitest on fixtures is not this.

The web stack is `verify-freehire`. This skill talks to production freehire.me: a `wxt
build` reads `extension/.env.production`, so the panel signs in and reads matches against
the owner's real account.

## Hard rules

- **Never press the final submit.** On LinkedIn it is the dialog button `Submit
  application`; on any ATS, the button that sends the application. The candidate presses it.
  Stop on the review screen and hand over.
- **One driver.** This is the owner's everyday browser. Do not run two verifications at
  once, and do not type while the owner is typing: foreground input moves their pointer.
- Any test value typed into a real application (a placeholder salary, say) is reported to
  the owner before handover, so it never goes out by accident.

## Helpers

| Command (from the repo root) | What it does |
|---|---|
| `.agents/skills/verify-extension/extension.sh build` | `wxt build`, copied to `~/.freehire-verify/extension` (stable path Chrome loads) |
| `.agents/skills/verify-extension/extension.sh doctor` | read-only: built version, unpacked + store entries in the profile, enabled state, path |
| `.agents/skills/verify-extension/extension.sh evidence` | creates and prints `~/.freehire-verify/evidence/<timestamp>/` |

Env: `FH_EXT_DIR` (build target), `FH_CHROME_PROFILE` (default `Profile 1`, the owner's),
`FH_EVIDENCE_DIR`.

## Launch

1. `extension.sh build`. Ready when it prints `built <version>`.
2. Chrome must already run (`cua_list_apps` → `com.google.Chrome`). Open the extensions page
   with `cua_launch_app {bundle_id:"com.google.Chrome", urls:["chrome://extensions/"]}`
   (typing into the address bar is refused in background mode). Confirm the window title ends
   with the owner's profile name, not another profile's.
3. First time only: press `Load unpacked`; the macOS Open panel appears as its own window.
   Send `cmd+shift+g` with `delivery_mode:"foreground"` (background keys go to the page),
   `cua_set_value` the `PathTextField` to `~/.freehire-verify/extension/` (typing appends to
   a remembered path), press Return in the foreground, then press `Select`. Chrome shows
   "Extension loaded" and a freehire card with ID `ebhfafmnihndlaoibgjnfjhkjlnaencj`.
   Later builds: press the reload arrow on that card instead.
4. Turn the store build off for the run: open
   `chrome://extensions/?id=ijfaechijopdlikalojadpojmpilplnj` and press the checkbox
   `On, extension enabled`. With both on, two freehire icons and panels compete.
5. `extension.sh doctor` must print `doctor OK` with `unpacked <built version> enabled=True`.

## Doctor

`extension.sh doctor` answers "is this the build I mean to test?". Then, in the panel
header, the version chip must read the built version (for example `v0.1.14`) and the status
chip `ready`; `offline` means not signed in.

## Drive (cua_* tools)

Recipe that held on Chrome 158, cua-driver 0.47:

- Snapshot with `cua_get_window_state`, but ALWAYS pass `query` and `max_elements` (≈200-800)
  or `include_accessibility_tree:false`: Chrome's AX tree carries the whole bookmarks and
  history menus (thousands of nodes) and an unbounded read floods the context.
- Page and side panel are two `AXWebArea`s in the same window: the employer page is titled
  after the tab, the panel `freehire`.
- Press by `element_token` (`AXPress`) for buttons; it works in the background.
- Text into a web field: element `cua_type_text`/`cua_set_value` writes AX only, which React
  may not see. For a real keystroke path click the field with pixel `x,y` taken from the
  screenshot image (the driver scales screenshot pixels to points) using
  `delivery_mode:"foreground", count:3` (select all), then `cua_type_text` foreground.
- Stable handles: toolbar `freehire\nHas access to this site` (opens the panel), panel
  `Sign in with freehire`, consent page button `Connect`, LinkedIn `Easy Apply to this job`,
  panel `Autofill`, dialog `Next` / `Review` / `Submit application`.

Feature journeys: [features/README.md](features/README.md).

## Evidence

Run `extension.sh evidence` first and save into that directory: a screenshot after each
action that changes state (panel notice, dialog step), named `NN-what.png`. Copy them from the
tool-results path the screenshot tool returns. A journey proves itself with the panel's own
words (for example `✓ Autofilled 1 field`), the dialog's step counter (`3/4 pages`) and
LinkedIn's validation text, not with "no error was shown". Side effects to check besides the
screen: `extension.sh doctor` (what is installed) and, once marking applied exists, the
application in freehire's tracker.

## Cleanup

Evidence stays in `~/.freehire-verify/evidence/`; nothing below deletes it.

- Leave an application the owner may still send open on its review screen and say so. To
  abandon one instead: dialog `Dismiss` → `Discard`. Never `Submit application`.
- Restore the browser only if the owner wants the store build back: press the store card's
  checkbox on again, and `Remove` the unpacked card. Otherwise leave the unpacked build in;
  it is what the owner asked for.
- Do not quit Chrome or close tabs you did not open.
