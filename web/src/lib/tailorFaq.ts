// CV-tailoring landing FAQ — the single source for both the visible section
// (TailorLandingView.svelte) and the FAQPage JSON-LD (routes/features/tailor).
// Google requires the schema's answers to match the on-page text, so they share
// this. Written around what stops a purchase, answer-first, and honest to what the
// product does (internal/candidate/cvedit, internal/candidate/experience).

import type { FaqItem } from './seo';

export const TAILOR_FAQ: FaqItem[] = [
  {
    question: 'Will a recruiter be able to tell an AI wrote it?',
    answer:
      'The words are yours to begin with. The agent reorders and rephrases what you already did, in the vacancy\'s own terms, one edit at a time — and you can see, change or undo each of them. There is no wholesale rewrite in a generic voice to give it away.',
  },
  {
    question: 'Can it put things on my CV that aren\'t true?',
    answer:
      'No. Before it writes anything, it splits the job\'s requirements into what your history covers and what it does not. The first it brings forward; the second it asks you about. Anything a model merely inferred is stored as the model\'s and cannot go into a CV until you confirm it — that check is in the code, not in a prompt.',
  },
  {
    question: 'What happens to my original CV?',
    answer:
      'Nothing. Tailoring copies it into a new CV bound to one vacancy, so every application gets its own version and your base CV is never edited.',
  },
  {
    question: 'Will an ATS read the PDF?',
    answer:
      'Yes. The CV is typeset into a single-column PDF with real text — no tables, no side columns, no words inside an image — so the parser reads what you see. Templates change the typography, never the content.',
  },
  {
    question: 'How much does it cost, and how do I cancel?',
    answer:
      'Tailoring is part of Pro, which also covers cover letters, the AI assistant and auto-apply. It is billed monthly and you can cancel any time from your plan page; Pro stays on until the end of the period you paid for.',
  },
  {
    question: 'What happens to my data?',
    answer:
      'Your CV is used only for the features you run on it. Your contact details are never sent to the tailoring agent, and the whole codebase is open source, so you can check how it is handled.',
  },
  {
    question: 'Can I drive it from my own agent or the terminal?',
    answer:
      'Yes. The freehire CLI runs the same loop with one API key — read the analysis, read the CV, apply one edit, render the PDF — under the same rules, so your agent cannot write in what you have not done either.',
  },
];
