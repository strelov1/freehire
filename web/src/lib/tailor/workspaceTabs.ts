// The tailoring workspace's tabs: the left panel's, and the one flat bar below lg.

// The left panel holds what CHANGES the document — its text, its template, its typography —
// and the chat that does all three by asking. Measuring the document is the right panel's job.
export type LeftTab = 'chat' | 'editor' | 'experience' | 'design';
export const leftTabs: [LeftTab, string][] = [
  ['chat', 'Chat'],
  ['editor', 'Editor'],
  ['experience', 'Experience'],
  ['design', 'Design'],
];

// Mobile-only navigation: below lg the three columns collapse to one, so a single flat tab bar
// picks which view fills the screen. At lg it's hidden and every column shows at once as before.
// mobileView is the sole source of truth for per-region visibility on mobile; picking a tab also
// syncs the matching column's own selector (mobile → column) so the wide layout shows the same
// content once revealed. The reverse (a desktop tab change updating mobileView) is not wired —
// switching a column tab then narrowing across lg resets the mobile view to that tab's default.
export type MobileView =
  | 'chat'
  | 'editor'
  | 'experience'
  | 'design'
  | 'preview'
  | 'jd'
  | 'jobmatch'
  | 'score'
  | 'letter'
  | 'history';
const allMobileTabs: [MobileView, string][] = [
  ['chat', 'Chat'],
  ['editor', 'Editor'],
  ['experience', 'Experience'],
  ['design', 'Design'],
  ['preview', 'Preview'],
  ['jobmatch', 'Job Match'],
  ['score', 'ATS'],
  ['letter', 'Letter'],
  ['history', 'History'],
  ['jd', 'Job'],
];
// The vacancy panes do not exist in base mode: only the history remains of the right panel.
export function mobileTabsFor(baseMode: boolean): [MobileView, string][] {
  return baseMode
    ? allMobileTabs.filter(([id]) => id !== 'jobmatch' && id !== 'score' && id !== 'letter' && id !== 'jd')
    : allMobileTabs;
}

/** Whether a mobile view is one of the left panel's tabs. */
export function isLeftView(v: MobileView): v is LeftTab {
  return v === 'chat' || v === 'editor' || v === 'experience' || v === 'design';
}
