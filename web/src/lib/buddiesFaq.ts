// Interview-buddies landing FAQ — the single source for both the visible section
// (BuddiesLandingView.svelte) and the FAQPage JSON-LD (routes/features/interview-buddies).
// Google requires the schema's answers to match the on-page text, so they share this.
// Every answer restates internal/engage/buddy/AGENTS.md and the interview-buddies spec;
// nothing here may promise more than those do.

import type { FaqItem } from './seo';

export const BUDDIES_FAQ: FaqItem[] = [
  {
    question: 'Will my employer or a recruiter see this?',
    answer:
      'Not as you. Buddy pages are visible only to signed-in members of the network — not to search engines and not in the public Talent Network catalogue. Anyone with an account can join, a recruiter included, but every member appears under a random handle, separate from your Talent Network handle and username.',
  },
  {
    question: 'What exactly do other members see about me?',
    answer:
      'Your handle, your role titles as you wrote them, and facts like your category, seniority, skills and country. In the feed, they see when you applied to a company and when you reached the interview or offer stage, by day. Never your name or email, and nothing else from your CV — so if a role title names your employer, edit it before joining. Never a rejection, a withdrawal, or the offer you accepted.',
  },
  {
    question: 'Can I keep one application out of the feed?',
    answer:
      'Yes. Each application you track has a switch to hide it from the buddy feed, and postings you pasted in privately never appear there at all.',
  },
  {
    question: "What if someone messages me in a way I don't like?",
    answer:
      'Block them: the conversation ends for good and you disappear from each other everywhere in the network. You can also report the conversation to our moderators. Declining an invitation is silent — the other person is not told.',
  },
  {
    question: 'Does it cost anything?',
    answer: 'No. Interview buddies is free for every signed-in account.',
  },
  {
    question: 'What happens if I leave?',
    answer:
      'You disappear from every list and feed, and your conversations close. If you come back later, you get the same handle back and old conversations can reopen.',
  },
  {
    question: 'Will I know when someone writes to me?',
    answer:
      'Yes. An unread message is announced by email (unless you turned activity emails off), Telegram if you linked it, and in the app. The notice says only that a message is waiting — never what it says.',
  },
];
