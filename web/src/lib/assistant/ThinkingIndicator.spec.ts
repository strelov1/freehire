import { render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import ThinkingIndicator from './ThinkingIndicator.svelte';

vi.mock('$app/state', () => ({
  page: { data: { locale: 'en' }, url: new URL('http://localhost/') },
}));

const VERBS = ['Thinking', 'Working', 'Crunching', 'Pondering', 'Composing', 'Simmering'];

describe('ThinkingIndicator', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('shows one of the working verbs and an elapsed counter that advances', () => {
    render(ThinkingIndicator);
    const status = screen.getByRole('status');

    expect(VERBS.some((v) => status.textContent?.includes(`${v}…`))).toBe(true);
    expect(status.textContent).toContain('(0s)');

    vi.advanceTimersByTime(2100);
    flushSync();
    expect(status.textContent).toContain('(2s)');
  });

  it('shows a label instead of a verb, without a counter', () => {
    render(ThinkingIndicator, { label: 'Waiting for your turn' });
    const status = screen.getByRole('status');

    expect(status.textContent).toContain('Waiting for your turn…');
    expect(status.textContent).not.toMatch(/\(\d+s\)/);
  });
});
