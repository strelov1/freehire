// Repository URLs and the install line for the CLI — shared by the visible links
// (CliView.svelte), the homepage's CLI card (HomeLandingView.svelte) and the
// SoftwareApplication JSON-LD (routes/cli), so `codeRepository` can never name a repo
// the page does not link to. The skill link lives in the view, which is its only
// consumer.

export const CLI_REPO = 'https://github.com/strelov1/freehire-cli';
export const MCP_REPO = 'https://github.com/strelov1/freehire-mcp';

/** What a visitor copies to install it, and what `web/static/install.sh` is served at.
 *  Lives here because two pages now offer it to the clipboard: a second spelling would
 *  be a second thing to keep true, and the one that drifted would fail silently — a
 *  copied command that 404s looks like the site is broken, not like a typo. */
export const CLI_INSTALL = 'curl -fsSL https://freehire.me/install.sh | sh';
