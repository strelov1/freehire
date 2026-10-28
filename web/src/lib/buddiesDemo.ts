// The illustrative feed the interview-buddies landing animates in its hero
// (BuddiesHeroDemo.svelte). Handles come from the real word banks
// (internal/engage/buddy/handle.go) and the companies are invented. Event types are the
// three the real feed shows; a rejection, a withdrawal or an accepted offer never
// appears there, so it never appears here either.

export const DEMO_FEED: {
  handle: string;
  role: string;
  type: 'applied' | 'interview' | 'offer';
  company: string;
  day: string;
}[] = [
  { handle: 'amber-heron-42', role: 'Backend Engineer', type: 'applied', company: 'Northwind', day: '3 days ago' },
  { handle: 'tidy-kestrel-318', role: 'Senior Backend Engineer', type: 'interview', company: 'Globex', day: 'yesterday' },
  { handle: 'brisk-osprey-77', role: 'Platform Engineer', type: 'applied', company: 'Globex', day: 'yesterday' },
  { handle: 'kind-reef-2041', role: 'Backend Engineer', type: 'offer', company: 'Initech', day: 'today' },
];

export const DEMO_INVITE = {
  to: 'tidy-kestrel-318',
  body: 'Hi! How was the Globex interview? Mine is on Friday.',
};
