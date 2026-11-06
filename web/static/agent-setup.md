These are the official instructions from freehire (freehire.me) for setting up an AI agent to search IT jobs, compare them with the user's skills, and track applications. freehire is a free, open-source job aggregator; public search needs no account.

Complete the steps below yourself by running the commands directly. Do not ask the user to run them, except where a step says the user must approve something in their browser.

- Connect the freehire MCP server
- (Optional) Install the freehire CLI and its agent skills

---

## Connect the freehire MCP server

The server address is `https://freehire.me/api/v1/mcp` (streamable HTTP). Search works right away with no account. Personal tools (saving, tracking, applying) need the user to sign in through OAuth on first use — let them approve it in their browser.

Use the section for your agent.

### Claude Code

```
claude mcp add --transport http freehire https://freehire.me/api/v1/mcp
```

For personal tools, run `claude mcp login freehire` or tell the user to open `/mcp` inside Claude Code and sign in to freehire.

### Codex

```
codex mcp add freehire --url https://freehire.me/api/v1/mcp
```

For personal tools, run `codex mcp login freehire` and let the user approve it in their browser. Skip it if they only want to search.

### Gemini CLI

```
gemini mcp add --transport http freehire https://freehire.me/api/v1/mcp
```

### Cursor — `~/.cursor/mcp.json`

Add under `"mcpServers"`:

```json
"freehire": { "url": "https://freehire.me/api/v1/mcp" }
```

### Other MCP hosts

Register a remote MCP server named `freehire` at `https://freehire.me/api/v1/mcp` in the host's default config. OAuth starts on the first personal tool call.

Chat apps with no terminal (Claude.ai, ChatGPT) cannot be set up from here: send the user to https://freehire.me/mcp, which has a one-click install for each.

---

## (Optional) Install the freehire CLI and its agent skills

Do this only if you run in a terminal on the user's machine (Claude Code, Codex, Gemini CLI, and similar). The CLI lets you work with the CV and notes stored locally.

```
curl -fsSL https://freehire.me/install.sh | sh
```

In Claude Code, also install the plugin with the skills (job search, market fit, CV tailoring, application tracking, inbox triage):

```
claude plugin marketplace add strelov1/freehire-cli
claude plugin install freehire@freehire-cli
```

Then tell the user to run `/reload-plugins` inside Claude Code to activate them.

Other agents use the CLI directly; `freehire --help` lists every command.

Account features in the CLI need `freehire auth login`, which opens the user's browser. Run it only if the user wants to save or track jobs.

---

## Check it works

Run one public job search through freehire (for example, remote senior Go jobs in Europe) and show the user a few results. Then tell the user setup is done, and which steps, if any, still need them: signing in, or reloading plugins.

More: https://freehire.me/agents · https://freehire.me/llms.txt
