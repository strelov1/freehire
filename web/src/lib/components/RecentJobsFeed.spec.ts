import { fireEvent, render, screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import RecentJobsFeed from './RecentJobsFeed.svelte';

vi.mock('$app/paths', () => ({ resolve: (path: string, params?: { slug: string }) => (params ? path.replace('[slug]', params.slug) : path) }));
vi.mock('#lib/i18n/currentLocale.svelte.js', () => ({ locale: () => 'en' }));
vi.mock('#lib/api.js', () => ({ api: { recentJobsFeedUrl: () => '/feed' } }));

// A hand-driven EventSource: the spec decides when a frame arrives.
let emit: (title: string) => void = () => {};
class FakeEventSource {
  constructor() {
    emit = () => {};
  }
  addEventListener(_type: string, fn: (e: MessageEvent) => void) {
    emit = (title) =>
      fn({ data: JSON.stringify({ kind: 'single', title, company_name: 'Acme', job_slug: title, produced_at: new Date().toISOString() }) } as MessageEvent);
  }
  close() {}
}

beforeEach(() => {
  vi.stubGlobal('EventSource', FakeEventSource);
  // jsdom implements no Web Animations; the list's fly/flip transitions call it.
  Element.prototype.animate ??= function animate() {
    return { cancel() {}, finish() {}, play() {}, onfinish: null, currentTime: 0 } as unknown as Animation;
  };
  // A card pushed out of a full list runs an outro, which asks for running animations.
  Element.prototype.getAnimations ??= function getAnimations() {
    return [];
  };
});

const titles = () => screen.getAllByRole('link').map((a) => a.textContent?.trim().split(/\s+/)[0]);

describe('RecentJobsFeed', () => {
  it('lays out every slot from the first arrival, so nothing below moves', async () => {
    const { container } = render(RecentJobsFeed);
    emit('one');
    await tick();
    // 1 card + 7 invisible placeholders.
    expect(container.querySelectorAll('ul > li')).toHaveLength(8);
    emit('two');
    await tick();
    expect(container.querySelectorAll('ul > li')).toHaveLength(8);
  });

  it('holds arrivals while paused and lands them on resume', async () => {
    render(RecentJobsFeed);
    emit('one');
    await tick();
    await fireEvent.click(screen.getByRole('button', { name: 'Pause' }));
    emit('two');
    await tick();
    expect(titles()).toEqual(['one']);
    const resume = screen.getByRole('button', { name: /Resume \(1 new\)/ });
    await fireEvent.click(resume);
    await tick();
    expect(titles()).toEqual(['two', 'one']);
  });

  it('holds while keyboard focus is inside the list', async () => {
    render(RecentJobsFeed);
    emit('one');
    await tick();
    const link = screen.getByRole('link');
    link.focus();
    await fireEvent.focusIn(link);
    emit('two');
    await tick();
    expect(titles()).toEqual(['one']);
    await fireEvent.focusOut(link, { relatedTarget: document.body });
    await tick();
    expect(titles()).toEqual(['two', 'one']);
  });

  it('counts every arrival during a pause, past the queue it keeps', async () => {
    render(RecentJobsFeed);
    emit('one');
    await tick();
    await fireEvent.click(screen.getByRole('button', { name: 'Pause' }));
    for (let i = 0; i < 12; i++) emit(`held${i}`);
    await tick();
    const resume = screen.getByRole('button', { name: /Resume \(12 new\)/ });
    await fireEvent.click(resume);
    await tick();
    // Only the newest eight can be shown, and the count resets once they land.
    expect(titles()[0]).toBe('held11');
    expect(screen.getByRole('button', { name: 'Pause' })).toBeTruthy();
  });
});
