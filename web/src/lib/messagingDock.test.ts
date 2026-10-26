import { describe, expect, it } from 'vitest';
import {
  closeDockWindow,
  dockCorner,
  dockHiddenOn,
  dockReserveClass,
  dockToastOffsetClass,
  DraftBook,
  mobileSheetId,
  openDockWindow,
  parseDockWindows,
  pruneDockWindows,
  setDockWindowMinimized,
  totalUnread,
} from './messagingDock';

describe('dock windows', () => {
  it('opens a window at the end and closes the oldest past two', () => {
    let windows = openDockWindow([], 1);
    windows = openDockWindow(windows, 2);
    expect(windows.map((w) => w.id)).toEqual([1, 2]);
    windows = openDockWindow(windows, 3);
    expect(windows.map((w) => w.id)).toEqual([2, 3]);
  });

  it('restores an open window in place instead of duplicating it', () => {
    let windows = openDockWindow(openDockWindow([], 1), 2);
    windows = setDockWindowMinimized(windows, 1, true);
    windows = openDockWindow(windows, 1);
    expect(windows).toEqual([{ id: 1, minimized: false }, { id: 2, minimized: false }]);
  });

  it('honours a smaller maximum', () => {
    expect(openDockWindow(openDockWindow([], 1, 1), 2, 1)).toEqual([{ id: 2, minimized: false }]);
  });

  it('closes and prunes by conversation id', () => {
    const windows = openDockWindow(openDockWindow([], 1), 2);
    expect(closeDockWindow(windows, 1).map((w) => w.id)).toEqual([2]);
    expect(pruneDockWindows(windows, new Set([2, 9])).map((w) => w.id)).toEqual([2]);
  });
});

describe('dock persistence and badge', () => {
  it('reads back only well-formed window ids', () => {
    expect(parseDockWindows(null)).toEqual([]);
    expect(parseDockWindows('not json')).toEqual([]);
    expect(parseDockWindows('{"id":1}')).toEqual([]);
    expect(parseDockWindows('[{"id":1,"minimized":true},{"id":1},{"id":-2},{"id":"3"},{"id":4,"body":"hi"}]'))
      .toEqual([{ id: 1, minimized: true }, { id: 4, minimized: false }]);
    expect(parseDockWindows('[{"id":1},{"id":2},{"id":3}]').map((w) => w.id)).toEqual([2, 3]);
  });

  it('sums unread messages across conversations', () => {
    expect(totalUnread([])).toBe(0);
    expect(totalUnread([{ unread: 2 }, { unread: 0 }, { unread: 5 }])).toBe(7);
  });
});

describe('dock placement', () => {
  it('stays off the Chats tab page only', () => {
    expect(dockHiddenOn('/my/buddies')).toBe(true);
    expect(dockHiddenOn('/my/buddies/some-handle')).toBe(false);
    expect(dockHiddenOn('/jobs')).toBe(false);
  });

  it('reports how much of the corner it takes', () => {
    expect(dockCorner(false, true, [])).toBe('none');
    expect(dockCorner(true, false, [])).toBe('bars');
    expect(dockCorner(true, false, [{ id: 1, minimized: true }])).toBe('bars');
    expect(dockCorner(true, false, [{ id: 1, minimized: false }])).toBe('open');
    expect(dockCorner(true, true, [])).toBe('open');
  });
});

describe('mobile sheet', () => {
  it('is the newest window that is not minimized', () => {
    expect(mobileSheetId([])).toBeNull();
    expect(mobileSheetId([{ id: 1, minimized: false }, { id: 2, minimized: false }])).toBe(2);
    expect(mobileSheetId([{ id: 1, minimized: false }, { id: 2, minimized: true }])).toBe(1);
    expect(mobileSheetId([{ id: 1, minimized: true }, { id: 2, minimized: true }])).toBeNull();
  });
});

describe('dock reserve', () => {
  it('reserves one bar on a wide screen and a row per minimized window on a narrow one', () => {
    expect(dockReserveClass(false, false, [])).toBe('');
    expect(dockReserveClass(true, true, [{ id: 1, minimized: true }])).toBe('lg:pb-12');
    expect(dockReserveClass(true, false, [])).toBe('pb-12');
    expect(dockReserveClass(true, false, [{ id: 1, minimized: false }])).toBe('pb-12');
    expect(dockReserveClass(true, false, [{ id: 1, minimized: true }])).toBe('pb-28 sm:pb-12');
    expect(dockReserveClass(true, false, [{ id: 1, minimized: true }, { id: 2, minimized: true }])).toBe('pb-44 sm:pb-12');
  });

  it('lifts the support toast above every stacked bar', () => {
    expect(dockToastOffsetClass([])).toBe('bottom-16');
    expect(dockToastOffsetClass([{ id: 1, minimized: true }])).toBe('bottom-32 sm:bottom-16');
    expect(dockToastOffsetClass([{ id: 1, minimized: true }, { id: 2, minimized: true }])).toBe('bottom-48 sm:bottom-16');
  });
});

describe('drafts', () => {
  it('keeps a draft per conversation and forgets an emptied one', () => {
    const drafts = new DraftBook();
    drafts.set(1, 'hello');
    drafts.set(2, 'other');
    expect(drafts.get(1)).toBe('hello');
    expect(drafts.get(2)).toBe('other');
    expect(drafts.get(3)).toBe('');
    drafts.set(1, '');
    expect(drafts.get(1)).toBe('');
    drafts.clear();
    expect(drafts.get(2)).toBe('');
  });
});
