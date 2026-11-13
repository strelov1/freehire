// Shared by the visible /agents FAQ and its FAQPage JSON-LD.
import type { FaqItem } from './seo';

export const AGENTS_FAQ: FaqItem[] = [
  {
    question: 'Which option should I choose?',
    answer:
      'Choose freehire Assistant to work directly on the site with Pro. Choose MCP to connect an assistant you already use, such as Claude, ChatGPT, Codex or Cursor. Choose CLI to work from your terminal or give a local agent access to freehire alongside your files. Choose API to build on the catalogue: the read API is free for personal and non-commercial use, and a Jobs API plan licenses the data for your product.',
  },
  {
    question: 'Do I need Pro or an account to get started?',
    answer:
      'The built-in freehire Assistant is included in Pro and uses your signed-in account. Public job search, employer data and market figures through the CLI need no freehire account. The hosted MCP asks you to sign in to freehire when you connect it, and the same connection then saves jobs, updates application stages and uses your profile. Your external assistant may have its own subscription requirements.',
  },
  {
    question: 'How do I connect my account?',
    answer:
      'For MCP, add the server address and approve OAuth in your browser when your assistant asks; the hosted MCP needs a freehire account. Codex uses codex mcp login freehire; Claude Code uses its /mcp authentication control. For the CLI, run freehire auth login. The built-in assistant uses your existing freehire sign-in.',
  },
  {
    question: 'Can my assistant submit applications for me?',
    answer:
      'Saving a job or marking it applied updates your tracking board and does not submit anything to an employer. For automatic applications, connect with OAuth and enable “Let connected agents apply for me” under Account → Integrations → API keys. Then explicitly ask the assistant to prepare an application on a supported employer form. You review the prepared application before approving submission. Availability depends on your plan and allowance. The browser extension can also fill a form for you to review and send.',
  },
  {
    question: 'Does each option use the same tracking board?',
    answer:
      'Yes. When connected to the same freehire account, the built-in assistant, MCP and CLI save jobs and update stages on the same board you see on the website.',
  },
];
