import { render } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { describe, expect, it } from 'vitest';
import Tabs from './tabs.svelte';
import { must } from './test-utils';

const slot = (html: string) => createRawSnippet(() => ({ render: () => html }));

const TABS = [
  { value: 'one', label: 'One' },
  { value: 'two', label: 'Two' },
  { value: 'three', label: 'Three' },
];

function setup(value?: string) {
  const { getAllByRole } = render(Tabs, {
    tabs: TABS,
    value,
    children: slot('<div>panel</div>'),
  });
  return getAllByRole('tab') as HTMLButtonElement[];
}

describe('Tabs', () => {
  // The regression that motivated this suite: keying the roving tabindex off
  // `value` left every trigger at -1 until the consumer picked one, so Tab
  // could not enter the tablist at all.
  it('makes the first tab focusable when no value is set', () => {
    const [first, second] = setup();

    expect(first?.tabIndex).toBe(0);
    expect(first?.getAttribute('aria-selected')).toBe('true');
    expect(second?.tabIndex).toBe(-1);
  });

  it('moves the selection with the arrow keys and takes focus along', async () => {
    const [first, second] = setup();

    first?.focus();
    await fireArrow(must(first), 'ArrowRight');

    expect(second?.getAttribute('aria-selected')).toBe('true');
    expect(second?.tabIndex).toBe(0);
    expect(document.activeElement).toBe(second);
  });

  it('wraps around both ends', async () => {
    const tabs = setup('one');

    await fireArrow(must(tabs[0]), 'ArrowLeft');
    expect(tabs[2]?.getAttribute('aria-selected')).toBe('true');

    await fireArrow(must(tabs[2]), 'ArrowRight');
    expect(tabs[0]?.getAttribute('aria-selected')).toBe('true');
  });

  it('leaves other keys to the browser', async () => {
    const tabs = setup('one');
    const event = new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true });

    tabs[0]?.dispatchEvent(event);

    expect(event.defaultPrevented).toBe(false);
    expect(tabs[0]?.getAttribute('aria-selected')).toBe('true');
  });

  it('points the panel at the selected tab', () => {
    const { getByRole, getAllByRole } = render(Tabs, {
      tabs: TABS,
      children: slot('<div>panel</div>'),
    });

    const panel = getByRole('tabpanel');
    const [first] = getAllByRole('tab');

    expect(panel.getAttribute('aria-labelledby')).toBe(first?.id);
  });
});

async function fireArrow(el: HTMLElement, key: 'ArrowLeft' | 'ArrowRight') {
  el.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }));
  await Promise.resolve();
}
