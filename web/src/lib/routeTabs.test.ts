import { describe, it, expect } from 'vitest';
import { activeRouteTab } from './routeTabs';

const TABS = [
  { id: 'board', href: '/my/tracking' },
  { id: 'list', href: '/my/tracking/list' },
  { id: 'pipeline', href: '/my/tracking/pipeline' },
] as const;

const on = (pathname: string) => activeRouteTab(pathname, TABS, 'board');

describe('activeRouteTab', () => {
  it('is the index tab on the index route', () => {
    expect(on('/my/tracking')).toBe('board');
  });

  it('is the tab whose own route it is', () => {
    expect(on('/my/tracking/list')).toBe('list');
  });

  // The index route is a prefix of every other tab's, so a bare startsWith would
  // report the index as active everywhere.
  it('prefers the longest matching route over the index', () => {
    expect(on('/my/tracking/pipeline')).toBe('pipeline');
  });

  it('stays on a tab through its own sub-routes', () => {
    expect(on('/my/tracking/list/archived')).toBe('list');
  });

  // The regression the shared helper exists for: a sibling route that merely starts
  // with a tab's path is not that tab.
  it('does not match a route that only shares a prefix with a tab', () => {
    expect(on('/my/tracking/listing')).toBe('board');
  });

  it('falls back on a sub-route no tab owns', () => {
    expect(on('/my/tracking/42/notes')).toBe('board');
  });

  it('falls back on an unrelated path', () => {
    expect(on('/my/profile')).toBe('board');
  });
});
