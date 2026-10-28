// The illustrative feed the interview-buddies landing animates in its hero
// (BuddiesHeroDemo.svelte). Handles come from the real word banks
// (internal/engage/buddy/handle.go) and the companies are invented. Event types are the
// three the real feed shows; a rejection, a withdrawal or an accepted offer never
// appears there, so it never appears here either. `summary` is the card summary the real
// feed carries, in the dictionaries' own ids, so BuddyIdentity words it as the product does.

export const DEMO_FEED: {
  handle: string;
  summary: { category: string; seniority: string; country: string };
  type: 'applied' | 'interview' | 'offer';
  company: string;
  day: string;
}[] = [
  { handle: 'amber-heron-42', summary: { category: 'backend', seniority: 'mid', country: 'de' }, type: 'applied', company: 'Northwind', day: '3 days ago' },
  { handle: 'tidy-kestrel-318', summary: { category: 'backend', seniority: 'senior', country: 'pl' }, type: 'interview', company: 'Globex', day: 'yesterday' },
  { handle: 'brisk-osprey-77', summary: { category: 'devops', seniority: 'mid', country: 'nl' }, type: 'applied', company: 'Globex', day: 'yesterday' },
  { handle: 'kind-reef-2041', summary: { category: 'backend', seniority: 'senior', country: 'us' }, type: 'offer', company: 'Initech', day: 'today' },
];

export const DEMO_INVITE = {
  to: 'tidy-kestrel-318',
  body: 'Hi! How was the Globex interview? Mine is on Friday.',
};

export const DEMO_REPLY = {
  from: 'tidy-kestrel-318',
  body: 'Went well! Happy to tell you what they asked.',
};
