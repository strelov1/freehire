import { render } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import TabStrip, { tabStripId } from './tab-strip.svelte';
import { must } from './test-utils';

const TABS = [
  { id: 'one', label: 'One' },
  { id: 'two', label: 'Two' },
  { id: 'three', label: 'Three' },
];

function setup(active = 'one') {
  const onSelect = vi.fn();
  const { getAllByRole, rerender } = render(TabStrip, {
    tabs: TABS,
    active,
    onSelect,
    label: 'Demo sections',
    panelId: 'demo-panel',
  });
  return { tabs: getAllByRole('tab') as HTMLButtonElement[], onSelect, rerender };
}

describe('TabStrip', () => {
  it('makes only the active tab reachable by Tab, the roving tabindex', () => {
    const { tabs } = setup('two');

    expect(tabs[0]?.tabIndex).toBe(-1);
    expect(tabs[1]?.tabIndex).toBe(0);
    expect(tabs[1]?.getAttribute('aria-selected')).toBe('true');
  });

  it('reports the selection to the caller rather than owning it', async () => {
    const { tabs, onSelect } = setup('one');

    await fireArrow(must(tabs[0]), 'ArrowRight');

    expect(onSelect).toHaveBeenCalledWith('two');
    // Controlled: the DOM has not moved on its own until the caller re-renders with the new active id.
    expect(tabs[0]?.getAttribute('aria-selected')).toBe('true');
  });

  it('wraps around both ends', async () => {
    const { tabs, onSelect, rerender } = setup('one');
    await fireArrow(must(tabs[0]), 'ArrowLeft');
    expect(onSelect).toHaveBeenCalledWith('three');

    // Controlled component: simulate the caller committing the selection before
    // checking the other end wraps too.
    await rerender({ tabs: TABS, active: 'three', onSelect, label: 'Demo sections', panelId: 'demo-panel' });
    await fireArrow(must(tabs[2]), 'ArrowRight');
    expect(onSelect).toHaveBeenCalledWith('one');
  });

  it('Home and End jump to the first and last tab', async () => {
    const { tabs, onSelect } = setup('two');

    await fireArrow(must(tabs[1]), 'Home');
    expect(onSelect).toHaveBeenCalledWith('one');

    await fireArrow(must(tabs[1]), 'End');
    expect(onSelect).toHaveBeenCalledWith('three');
  });

  it('leaves other keys to the browser', () => {
    const { tabs, onSelect } = setup('one');
    const event = new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true });

    tabs[0]?.dispatchEvent(event);

    expect(event.defaultPrevented).toBe(false);
    expect(onSelect).not.toHaveBeenCalled();
  });

  it('points each tab id at its panel through the shared helper', () => {
    const { tabs } = setup('one');

    expect(tabs[0]?.id).toBe(tabStripId('demo-panel', 'one'));
    expect(tabs[0]?.getAttribute('aria-controls')).toBe('demo-panel');
  });

  it('names the strip for assistive tech', () => {
    const { getByRole } = render(TabStrip, {
      tabs: TABS,
      active: 'one',
      onSelect: vi.fn(),
      label: 'Profile sections',
      panelId: 'demo-panel',
    });

    expect(getByRole('tablist').getAttribute('aria-label')).toBe('Profile sections');
  });
});

async function fireArrow(el: HTMLElement, key: 'ArrowLeft' | 'ArrowRight' | 'Home' | 'End') {
  el.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }));
  await Promise.resolve();
}
