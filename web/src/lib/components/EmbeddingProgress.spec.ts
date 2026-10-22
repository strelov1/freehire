import { render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import type { EmbedProgress } from '#lib/types.js';
import EmbeddingProgress from './EmbeddingProgress.svelte';

/** A stand-in for the browser's EventSource that the test fires by hand. */
class FakeEventSource {
  static last: FakeEventSource | null = null;
  url: string;
  closed = false;
  private listeners: Record<string, ((e: MessageEvent) => void)[]> = {};
  constructor(url: string) {
    this.url = url;
    FakeEventSource.last = this;
  }
  addEventListener(type: string, fn: (e: MessageEvent) => void) {
    (this.listeners[type] ??= []).push(fn);
  }
  close() {
    this.closed = true;
  }
  emit(type: string, data: unknown) {
    for (const fn of this.listeners[type] ?? []) fn(new MessageEvent(type, { data: JSON.stringify(data) }));
  }
}

const unmeasured: EmbedProgress = {
  eligible: null,
  covered: null,
  snapshot_covered: null,
  queued: null,
  rate_per_minute: null,
  eta_seconds: null,
  snapshot_at: null,
  measured_at: null,
};

const measured: EmbedProgress = {
  eligible: 1_000_000,
  covered: 900_000,
  snapshot_covered: 880_000,
  queued: 100_000,
  rate_per_minute: 500,
  eta_seconds: 12_000,
  snapshot_at: '2026-10-19T09:00:00Z',
  measured_at: '2026-10-19T12:00:00Z',
};

beforeEach(() => {
  (globalThis as unknown as { EventSource: unknown }).EventSource = FakeEventSource;
});
afterEach(() => {
  FakeEventSource.last = null;
});

describe('EmbeddingProgress', () => {
  it('renders unmeasured figures as "measuring…", never as 0', () => {
    render(EmbeddingProgress, { initial: unmeasured, streamUrl: '/stream' });
    expect(screen.getAllByText('measuring…').length).toBeGreaterThanOrEqual(4);
    expect(screen.queryByText(/^0$/)).toBeNull();
    expect(screen.queryByText(/0(\.0)?%/)).toBeNull();
  });

  it('renders the measured figures', () => {
    render(EmbeddingProgress, { initial: measured, streamUrl: '/stream' });
    expect(screen.getByText('90%')).toBeTruthy();
    expect(screen.getByText('900,000')).toBeTruthy();
    expect(screen.getByText('100,000')).toBeTruthy();
    expect(screen.getByText('500 / min')).toBeTruthy();
    expect(screen.getByText('~3 h 20 min')).toBeTruthy();
  });

  it('replaces the figures when a progress event arrives', async () => {
    render(EmbeddingProgress, { initial: measured, streamUrl: '/stream' });
    await waitFor(() => expect(FakeEventSource.last?.url).toBe('/stream'));
    const source = FakeEventSource.last;
    if (!source) throw new Error('no EventSource opened');

    source.emit('progress', { ...measured, covered: 950_000, queued: 50_000 });

    await waitFor(() => expect(screen.getByText('95%')).toBeTruthy());
    expect(screen.getByText('50,000')).toBeTruthy();
  });

  it('links the volunteer client', () => {
    render(EmbeddingProgress, { initial: measured, streamUrl: '/stream' });
    const links = screen.getAllByRole('link').map((a) => a.getAttribute('href'));
    expect(links).toContain('https://github.com/strelov1/freehire-volunteer-embed');
    expect(links).toContain(
      'https://github.com/strelov1/freehire-volunteer-embed/blob/main/HOW_TO_RUN_THE_CLIENT.md',
    );
  });
});
