import { flushSync } from 'svelte';
import { render } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import Announcer from './components/Announcer.svelte';
import { ANNOUNCE_DELAY_MS, announce } from './announce.svelte.js';

describe('announce', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  function regions() {
    const { container } = render(Announcer);
    const polite = container.querySelector('[aria-live="polite"]') as HTMLElement;
    const assertive = container.querySelector('[aria-live="assertive"]') as HTMLElement;
    return { polite, assertive };
  }

  function settle() {
    vi.advanceTimersByTime(ANNOUNCE_DELAY_MS);
    flushSync();
  }

  it('speaks a polite message through the polite region only', () => {
    const { polite, assertive } = regions();
    announce('Saved');
    settle();
    expect(polite.textContent).toBe('Saved');
    expect(assertive.textContent).toBe('');
  });

  it('speaks an error through the assertive region', () => {
    const { polite, assertive } = regions();
    announce('Could not save', 'assertive');
    settle();
    expect(assertive.textContent).toBe('Could not save');
    expect(polite.textContent).not.toBe('Could not save');
  });

  it('clears the region before repeating an identical message', () => {
    const { polite } = regions();
    announce('Copied');
    settle();
    expect(polite.textContent).toBe('Copied');

    announce('Copied');
    flushSync();
    // The empty value in between is the mutation a screen reader needs to read it again.
    expect(polite.textContent).toBe('');
    settle();
    expect(polite.textContent).toBe('Copied');
  });

  it('speaks every message that arrives inside the delay, none dropped', () => {
    const { polite } = regions();
    announce('Saved');
    announce('Moved to Interview');
    settle();
    expect(polite.textContent).toBe('Saved. Moved to Interview.');
  });

  it('starts afresh once the joined message has landed', () => {
    const { polite } = regions();
    announce('First');
    announce('Second');
    settle();
    announce('Third');
    settle();
    expect(polite.textContent).toBe('Third');
  });
});
