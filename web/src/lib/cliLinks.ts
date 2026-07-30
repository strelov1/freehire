// Repository URLs for the CLI page — shared by the visible links (CliView.svelte)
// and the SoftwareApplication JSON-LD (routes/cli), so `codeRepository` can never
// name a repo the page does not link to. The install one-liner and skill link live
// in the view; only these two are needed in both places.

export const CLI_REPO = 'https://github.com/strelov1/freehire-cli';
export const MCP_REPO = 'https://github.com/strelov1/freehire-mcp';
