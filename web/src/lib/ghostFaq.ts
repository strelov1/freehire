// Ghost-jobs landing FAQ — the single source for both the visible section
// (GhostLandingView.svelte) and the FAQPage JSON-LD (routes/features/ghost-jobs).
// Google drops the rich result when the schema and the page disagree, so they share
// this array. Answers are written from the code (internal/ghost, internal/jobreality,
// internal/ghostreport), answer-first, and never claim more than the system observes.

import type { FaqItem } from './seo';

export const GHOST_FAQ: FaqItem[] = [
  {
    question: 'What is a ghost job?',
    answer:
      'A posting nobody is actually working to fill — kept up to collect CVs, to look like the company is growing, or simply because nobody took it down. Studies put it between 18% and 27% of listings; Greenhouse, which can see its own customers\' hiring pipelines, reported 18–22% per quarter with 70% of its employers having listed at least one.',
  },
  {
    question: 'Does freehire say a company is lying?',
    answer:
      'No, and it is built so that it cannot. The system observes two things: how a posting behaves, and what happened to people who applied. It never observes intent, so the strongest thing it will say is that a posting may be inactive — always beside the specific facts that led there. "Open 240 days, found only on an aggregator" is checkable; "this employer is not really hiring" is a claim about someone\'s state of mind.',
  },
  {
    question: 'How many signals does it take before I see anything?',
    answer:
      'Two. A single signal is weak on its own — a genuinely hard senior role stays open a long time, and a company can be missing from our board coverage for reasons of its own — so one criterion alone shows nothing at all. You always see how many fired out of how many were checked.',
  },
  {
    question: 'Why does it sometimes say "no data" instead of a verdict?',
    answer:
      'Because that is the honest answer, and it tells you why the warning is not stronger. A posting flagged on two structural criteria with nothing known about applicants is a very different thing from one several people reported. Hiding the empty rows would leave you guessing which of the two you are looking at.',
  },
  {
    question: 'Do you use my applications to flag jobs for other people?',
    answer:
      'Only as an anonymous count, and only above a threshold. Nothing you wrote, no dates, no identity — and no count at all until at least two different people have contributed, because a count of one would point straight at the single person who applied. Below that threshold the number is absent from the response, not hidden in it.',
  },
  {
    question: 'An unanswered application means the job is fake, then?',
    answer:
      'No — that is why it takes two people and a second signal. Recruiters go quiet for ordinary reasons, and a silence only counts at all if your mailbox is connected, so a reply would have been seen. Where we cannot observe replies we count nothing, since a gap in our data is not an employer ignoring you.',
  },
  {
    question: 'I applied and never heard back. How do I report it?',
    answer:
      'Open the job, choose Report, and pick "No response". It asks one thing: when you applied. That date is what separates a real silence from impatience, and it is what makes your report useful to the next person. You can withdraw it if the employer eventually answers.',
  },
  {
    question: 'Does a flagged job get hidden or pushed down?',
    answer:
      'Neither. It keeps its place in search and in every list, and the mark disappears when the posting closes. The point is to tell you before you spend an hour on an application, not to make the decision for you.',
  },
];
