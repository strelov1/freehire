import { describe, it, expect, vi, afterEach } from 'vitest';
import { respondTo, ToolChannel, WS_SUBPROTOCOL_MARKER, type ChannelStatus } from './client';
import type { PageBridge } from './executor';
import { must } from '../test-utils';

const page: PageBridge = {
  readPage: async () => ({ url: 'https://example.test/', title: '', headline: '', text: '' }),
  readForm: async () => ({ fields: [], uploads: [] }),
  fillSimple: async (fills) => fills.map((f) => ({ label: f.label, status: 'filled' as const })),
  combobox: async () => ({ status: 'not_found' }),
};

describe('respondTo', () => {
  it('answers a tool call with a result frame correlated by id', async () => {
    const out = await respondTo('{"id":"x1","tool":"read_form"}', page);

    expect(JSON.parse(must(out))).toEqual({ id: 'x1', result: { fields: [], uploads: [] } });
  });

  it('answers a failing call with an error frame rather than nothing', async () => {
    const out = await respondTo('{"id":"x2","tool":"nope"}', page);

    expect(JSON.parse(must(out))).toMatchObject({ id: 'x2', error: expect.stringMatching(/unknown tool/i) });
  });

  it('stays silent on a frame it cannot correlate to a call', async () => {
    expect(await respondTo('garbage', page)).toBeNull();
    expect(await respondTo('{"tool":"read_form"}', page)).toBeNull();
  });
});

/** Stands in for the browser's WebSocket: the test drives its lifecycle events by hand. */
class FakeWebSocket {
  static instances: FakeWebSocket[] = [];
  static readonly OPEN = 1;

  onopen: (() => void) | null = null;
  onclose: (() => void) | null = null;
  onmessage: ((ev: MessageEvent) => void) | null = null;
  onerror: (() => void) | null = null;
  readyState = 0;
  sent: string[] = [];

  constructor(
    public url: string,
    public protocols: string[],
  ) {
    FakeWebSocket.instances.push(this);
  }

  send(data: string) {
    this.sent.push(data);
  }

  close() {
    this.readyState = 3;
  }

  triggerOpen() {
    this.readyState = FakeWebSocket.OPEN;
    this.onopen?.();
  }

  triggerClose() {
    this.onclose?.();
  }
}

function lastSocket(): FakeWebSocket {
  const s = FakeWebSocket.instances.at(-1);
  if (!s) throw new Error('no socket was opened');
  return s;
}

// Never exercised: no ToolChannel test here sends the channel a tool call.
const unusedPage: PageBridge = {
  readPage: () => Promise.reject(new Error('unused')),
  readForm: () => Promise.reject(new Error('unused')),
  fillSimple: () => Promise.reject(new Error('unused')),
  combobox: () => Promise.reject(new Error('unused')),
};

afterEach(() => {
  FakeWebSocket.instances = [];
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe('ToolChannel', () => {
  it('opens the relay socket carrying the marker and the token as subprotocols', () => {
    vi.stubGlobal('WebSocket', FakeWebSocket as unknown as typeof WebSocket);
    new ToolChannel(unusedPage).start('tok-1');

    expect(lastSocket().protocols).toEqual([WS_SUBPROTOCOL_MARKER, 'tok-1']);
  });

  it('reports connecting then open as the handshake completes', () => {
    vi.stubGlobal('WebSocket', FakeWebSocket as unknown as typeof WebSocket);
    const statuses: ChannelStatus[] = [];
    new ToolChannel(unusedPage, (s) => statuses.push(s)).start('tok-1');
    lastSocket().triggerOpen();

    expect(statuses).toEqual(['connecting', 'open']);
  });

  it('retries a dropped connection with a widening backoff, capped at 30s', () => {
    vi.useFakeTimers();
    vi.stubGlobal('WebSocket', FakeWebSocket as unknown as typeof WebSocket);
    new ToolChannel(unusedPage).start('tok-1');
    expect(FakeWebSocket.instances).toHaveLength(1);

    lastSocket().triggerClose();
    vi.advanceTimersByTime(999);
    expect(FakeWebSocket.instances).toHaveLength(1);
    vi.advanceTimersByTime(1);
    expect(FakeWebSocket.instances).toHaveLength(2);

    lastSocket().triggerClose();
    vi.advanceTimersByTime(1999);
    expect(FakeWebSocket.instances).toHaveLength(2);
    vi.advanceTimersByTime(1);
    expect(FakeWebSocket.instances).toHaveLength(3);
  });

  it('resets the backoff after a connection succeeds', () => {
    vi.useFakeTimers();
    vi.stubGlobal('WebSocket', FakeWebSocket as unknown as typeof WebSocket);
    new ToolChannel(unusedPage).start('tok-1');

    lastSocket().triggerClose(); // 1st failure: next delay 1s
    vi.advanceTimersByTime(1000);
    lastSocket().triggerOpen(); // succeeds: backoff resets
    lastSocket().triggerClose(); // 1st failure again: next delay should be 1s, not 2s

    vi.advanceTimersByTime(999);
    expect(FakeWebSocket.instances).toHaveLength(2);
    vi.advanceTimersByTime(1);
    expect(FakeWebSocket.instances).toHaveLength(3);
  });

  it('stops retrying once stop() is called', () => {
    vi.useFakeTimers();
    vi.stubGlobal('WebSocket', FakeWebSocket as unknown as typeof WebSocket);
    const channel = new ToolChannel(unusedPage);
    channel.start('tok-1');
    lastSocket().triggerClose();
    channel.stop();

    vi.advanceTimersByTime(60_000);
    expect(FakeWebSocket.instances).toHaveLength(1);
  });

  // The WebSocket API never exposes the HTTP status a rejected handshake got, so a
  // permanently dead token (expired, revoked) looks identical to a momentary network
  // blip from here — just an onclose with nothing to act on. Silence forever was the
  // actual production bug this closes: the channel already retries indefinitely
  // (right, for a restarting relay), but nothing ever told the user why autofill and
  // the browse tool stayed broken the whole time.
  it('reports unreachable after repeated failures, without breaking off the retry loop', () => {
    vi.useFakeTimers();
    vi.stubGlobal('WebSocket', FakeWebSocket as unknown as typeof WebSocket);
    const statuses: ChannelStatus[] = [];
    new ToolChannel(unusedPage, (s) => statuses.push(s)).start('tok-1');

    for (let i = 0; i < 5; i++) {
      lastSocket().triggerClose();
      vi.runOnlyPendingTimers();
    }
    expect(statuses.filter((s) => s === 'unreachable')).toHaveLength(1);
    expect(FakeWebSocket.instances.length).toBeGreaterThan(5);

    // Keeps retrying afterwards, and does not repeat the notice.
    lastSocket().triggerClose();
    vi.runOnlyPendingTimers();
    expect(statuses.filter((s) => s === 'unreachable')).toHaveLength(1);
  });

  it('can report unreachable again after a fresh start', () => {
    vi.useFakeTimers();
    vi.stubGlobal('WebSocket', FakeWebSocket as unknown as typeof WebSocket);
    const statuses: ChannelStatus[] = [];
    const channel = new ToolChannel(unusedPage, (s) => statuses.push(s));

    channel.start('tok-1');
    for (let i = 0; i < 5; i++) {
      lastSocket().triggerClose();
      vi.runOnlyPendingTimers();
    }
    expect(statuses.filter((s) => s === 'unreachable')).toHaveLength(1);

    channel.stop();
    statuses.length = 0;
    channel.start('tok-2'); // e.g. the user signed out and back in with a fresh token
    for (let i = 0; i < 5; i++) {
      lastSocket().triggerClose();
      vi.runOnlyPendingTimers();
    }
    expect(statuses.filter((s) => s === 'unreachable')).toHaveLength(1);
  });
});
