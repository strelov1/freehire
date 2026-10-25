import { describe, expect, it } from 'vitest';
import { messageActionFor } from './buddies';
import {
  closeDockWindow,
  dockCorner,
  dockHiddenOn,
  dockIntentFor,
  openDockWindow,
  parseDockWindows,
  pruneDockWindows,
  setDockWindowMinimized,
  totalUnread,
} from './buddyDock';

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

describe('dock intent', () => {
  const conv = (id: number, handle: string, status_for_me: 'incoming' | 'outgoing' | 'open' | 'closed', former_member = false) =>
    ({ id, other: { handle }, status_for_me, former_member });
  const conversations = [
    conv(1, 'open-one', 'open'), conv(2, 'sent-one', 'outgoing'), conv(3, 'asked-me', 'incoming'),
    conv(4, 'gone', 'closed', true), conv(5, 'came-back', 'closed'),
  ];

  it('opens a window for any existing conversation, pending or not', () => {
    expect(dockIntentFor(messageActionFor(conversations, 'open-one'))).toEqual({ kind: 'window', conversationId: 1 });
    expect(dockIntentFor(messageActionFor(conversations, 'sent-one'))).toEqual({ kind: 'window', conversationId: 2 });
    expect(dockIntentFor(messageActionFor(conversations, 'asked-me'))).toEqual({ kind: 'window', conversationId: 3 });
  });

  it('asks for a first message when there is none, and does nothing for a former member', () => {
    expect(dockIntentFor(messageActionFor(conversations, 'stranger'))).toEqual({ kind: 'invite' });
    expect(dockIntentFor(messageActionFor(conversations, 'gone'))).toBeNull();
    expect(dockIntentFor(messageActionFor(conversations, 'came-back'))).toEqual({ kind: 'invite' });
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
