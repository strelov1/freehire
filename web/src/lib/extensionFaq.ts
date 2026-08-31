// Browser-extension landing FAQ — the single source for both the visible section
// (ExtensionLandingView.svelte) and the FAQPage JSON-LD
// (routes/features/extension). Google drops the rich result when the two
// disagree, so they share this array.
//
// Answers are written answer-first and stay honest to what the extension does
// (see extension/AGENTS.md): the agent decides when to read, the filler engages
// only on a page that looks like a real application, and the walk down the form
// is the audit rather than something bolted on after it.

import type { FaqItem } from './seo';

export const EXTENSION_FAQ: FaqItem[] = [
  {
    question: 'Which browsers does it work in?',
    answer:
      'Chrome, and the Chromium browsers that carry its side panel. It is a Manifest V3 extension built around `chrome.sidePanel`, so Firefox and Safari are not supported today.',
  },
  {
    question: 'Do I need a freehire account?',
    answer:
      'Yes. The panel signs in with your freehire account and the agent works from your profile — the match card scores against your CV, and Autofill answers from what your profile already says. Signing up is free.',
  },
  {
    question: 'Can it read pages I have not asked it about?',
    answer:
      'No. Nothing is read in the background: a page read happens because a question you asked needed it, it travels over a channel that exists only while the side panel is open, and it is named in the conversation when it happens. The extension also refuses any tab that is not an http or https page, decided from the address before the page is touched.',
  },
  {
    question: 'Does it submit applications for me?',
    answer:
      'No. It fills the form and stops. Autofill walks the questions one at a time, scrolling to each and outlining it as the answer lands, so you watch it happen rather than proofread a form that was filled in one jump. Pressing Submit is yours.',
  },
  {
    question: 'Which application forms does it handle?',
    answer:
      'It reads the form that is actually on the page rather than matching a list of vendors, so Greenhouse, Lever, Workday, Ashby, iCIMS, SmartRecruiters and Recruitee all work — and so does a career page nobody has heard of. Custom dropdowns that are not real select elements are driven too.',
  },
  {
    question: 'Will it fill in a form that is not an application?',
    answer:
      'It will not. The filler only engages on a page carrying the marks of a real application, a CV upload among them, which is what stops a newsletter box or a job-alert signup from being written into. The checklist of questions is shown on a looser test, but nothing is ever typed on the strength of it.',
  },
  {
    question: 'Does it work on jobs that are not in the freehire catalogue?',
    answer:
      'Yes. Open the panel on any posting and the agent reads that page and scores it against your profile. The actions that need a catalogue entry — saving the job, running the full AI match analysis — are hidden for a page freehire does not know, rather than offered and then failing.',
  },
  {
    question: 'Where does what it reads end up?',
    answer:
      'In the conversation, on freehire, under your account — one origin and nowhere else. You can read that conversation on the web and delete it; deleting it starts the panel on a fresh one.',
  },
  {
    question: 'What does it cost?',
    answer:
      'The extension is free. The agent draws on the same daily allowances the rest of freehire runs on: every plan can do every AI feature, and how much of each you can do in a day is what differs. It starts over every night.',
  },
];
