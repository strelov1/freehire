// FAQ for the /agents landing — the single source for both the visible section
// (AgentsLandingView.svelte) and the FAQPage JSON-LD. Google drops the rich result
// when the two disagree, so they share this array. Mirrors homeFaq.ts.
//
// Answers stay honest to what the surfaces actually do: public reads need no key,
// anything touching an account needs one, and the ranking of the three tracks is a
// consequence of what each host can run — not a preference.

import type { FaqItem } from './seo';

export const AGENTS_FAQ: FaqItem[] = [
  {
    question: 'Do I need an API key to let an agent search freehire?',
    answer:
      'No. Job search, company data and facets are public and need no authentication, so an agent can search the whole catalogue the moment it is installed. A key is only needed for the parts that belong to your account: saving, applying, application stages, notes, CV tailoring and application mail.',
  },
  {
    question: 'Which kind of agent works best with freehire?',
    answer:
      'A local harness — Claude Code, Codex, Cursor, Cline and the like — driving the freehire CLI. It gets the whole surface, including CV edits and PDF rendering, it can read and write files on your machine, and nothing caps how many tool calls one task may make. Hosted assistants come next: an MCP host such as Claude Desktop runs the same tools over the Model Context Protocol.',
  },
  {
    question: 'How do I connect freehire to Claude?',
    answer:
      'Two ways. In Claude Code, install the CLI and its agent skills, or add the plugin marketplace. In Claude Desktop, add the freehire MCP server — it runs via npx, needs no global install, and reuses the CLI credentials if you have already signed in.',
  },
  {
    question: 'Can ChatGPT use freehire?',
    answer:
      'Yes. There is a published custom GPT wired to the freehire API, and searching in it needs no setup. To save, apply and track, paste your own API key into the GPT authentication field. You can also import the OpenAPI schema into an Action of your own.',
  },
  {
    question: 'Can my agent apply to jobs for me?',
    answer:
      'No, and that is deliberate. Applications are submitted on the employer’s own site by you; freehire records that you applied, moves the application through its stages and reads the replies. The browser extension can fill an application form for you to review, but it never submits one on its own.',
  },
  {
    question: 'Is there a rate limit for agents?',
    answer:
      'Yes, and every response reports where you stand in the X-RateLimit-Remaining header. Ordinary reads get 600 requests per minute and the agent search endpoint gets 300. Read the header and pace yourself rather than waiting for a 429.',
  },
];
