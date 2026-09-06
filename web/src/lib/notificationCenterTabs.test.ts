import { describe, it, expect } from 'vitest';
import { NOTIFICATION_TABS } from './notificationCenterTabs';

describe('NOTIFICATION_TABS', () => {
  it('has one entry per tab id in display order', () => {
    expect(NOTIFICATION_TABS.map((t) => t.id)).toEqual(['history', 'searches', 'settings']);
  });

  // History is the index route, so it is a path-prefix of the other two — the case
  // activeRouteTab's longest-match rule exists for. Asserted against the real tabs
  // because the rule is only interesting over data shaped like this.
  it('nests the other tabs under the history route', () => {
    for (const tab of NOTIFICATION_TABS.filter((t) => t.id !== 'history')) {
      expect(tab.href.startsWith('/my/notifications/')).toBe(true);
    }
  });
});
