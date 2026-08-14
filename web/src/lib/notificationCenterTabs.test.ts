import { describe, it, expect } from 'vitest';
import { activeNotificationTab, NOTIFICATION_TABS } from './notificationCenterTabs';

describe('activeNotificationTab', () => {
  it('is history on the landing route', () => {
    expect(activeNotificationTab('/my/notifications')).toBe('history');
  });

  it('is searches on the search-alerts route', () => {
    expect(activeNotificationTab('/my/notifications/searches')).toBe('searches');
  });

  it('is settings on the settings route', () => {
    expect(activeNotificationTab('/my/notifications/settings')).toBe('settings');
  });

  it('falls back to history on the digest detail sub-route', () => {
    expect(activeNotificationTab('/my/notifications/42/jobs')).toBe('history');
  });

  it('falls back to history on an unrelated path', () => {
    expect(activeNotificationTab('/my/profile')).toBe('history');
  });
});

describe('NOTIFICATION_TABS', () => {
  it('has one entry per tab id in display order', () => {
    expect(NOTIFICATION_TABS.map((t) => t.id)).toEqual(['history', 'searches', 'settings']);
  });
});
