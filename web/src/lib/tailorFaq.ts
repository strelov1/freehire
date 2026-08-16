// CV-tailoring landing FAQ — the single source for both the visible section
// (TailorLandingView.svelte) and the FAQPage JSON-LD (routes/features/tailor).
// Google requires the schema's answers to match the on-page text, so they share
// this. Answers are written answer-first and stay honest to what the product
// does (internal/cv, internal/experience, internal/matchanalysis).

import type { FaqItem } from './seo';

export const TAILOR_FAQ: FaqItem[] = [
  {
    question: 'Does it invent experience I do not have?',
    answer:
      'No, and the design is what stops it rather than a promise. The tailoring context splits the job\'s requirements in two: the ones your history already covers but your CV buries, which the agent reframes, and the ones it does not, which the agent has to ask you about. Anything a model merely inferred is marked as such and cannot be written into a CV until you confirm it — the check lives in the service, not in a prompt.',
  },
  {
    question: 'How do I start tailoring?',
    answer:
      'From a job. Run the AI match analysis on a vacancy, then tailor from its result — the analysis is what the tailored CV reframes toward, so the flow needs one first. freehire copies your base CV into a new one bound to that vacancy; your original is never edited.',
  },
  {
    question: 'What does it actually change?',
    answer:
      'One field at a time: the summary, a header field, a bullet, the order of bullets within a role, a skill group, or a role\'s technology line. Each edit is a single operation you can see and undo, not a wholesale rewrite you have to proofread against the original.',
  },
  {
    question: 'Is the PDF ATS-friendly?',
    answer:
      'Yes. The CV renders through a typesetting engine into a clean, single-column PDF with real text — no tables, no columns, no images behind the words — so a parser reads the same content you see. Pick from the template gallery; switching templates never touches the content.',
  },
  {
    question: 'Can I do this from the terminal?',
    answer:
      'Yes. The freehire CLI drives the same flow: `cv context` prints the analysis to reframe toward, `cv get` dumps the document, `cv edit` applies one patch, and `cv render` downloads the PDF. Your own agent can run the whole loop with an API key.',
  },
  {
    question: 'What does it cost?',
    answer:
      'AI credits, the same currency the match analysis uses. Every account gets a monthly grant, and contributing a company board we do not track yet earns more. Your balance and what each action spent are on your credits page.',
  },
];
