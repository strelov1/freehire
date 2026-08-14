import { fireEvent, render } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { describe, expect, it, vi } from 'vitest';
import Tooltip from './tooltip.svelte';
import { must } from './test-utils';

const slot = (html: string) => createRawSnippet(() => ({ render: () => html }));

function setup() {
  const { container, queryByRole } = render(Tooltip, {
    children: slot('<button type="button">Salary</button>'),
    content: slot('<span>Median for the role</span>'),
  });
  const trigger = must(container.querySelector('button'));
  return { wrapper: must(trigger.parentElement), trigger, queryByRole };
}

describe('Tooltip', () => {
  it('shows nothing until the trigger is hovered or focused', () => {
    const { trigger, queryByRole } = setup();

    expect(queryByRole('tooltip')).toBeNull();
    expect(trigger.getAttribute('aria-describedby')).toBeNull();
  });

  // The regression: role="tooltip" associates nothing on its own, so without
  // this link the tooltip was invisible to a screen reader.
  it('describes the trigger while it is open', async () => {
    const { wrapper, trigger, queryByRole } = setup();

    await fireEvent.mouseEnter(wrapper);

    const tooltip = must(queryByRole('tooltip'));
    expect(tooltip.id).toBeTruthy();
    expect(trigger.getAttribute('aria-describedby')).toBe(tooltip.id);
  });

  it('opens on focus too, not only on hover', async () => {
    const { wrapper, trigger, queryByRole } = setup();

    await fireEvent.focusIn(wrapper);

    expect(queryByRole('tooltip')).not.toBeNull();
    expect(trigger.getAttribute('aria-describedby')).toBeTruthy();
  });

  // Leaving the attribute behind would point at an id that no longer exists,
  // which reads as no description at all.
  it('drops the description again once the close delay elapses', async () => {
    vi.useFakeTimers();
    try {
      const { wrapper, trigger, queryByRole } = setup();

      await fireEvent.mouseEnter(wrapper);
      await fireEvent.mouseLeave(wrapper);
      // The hide is deferred (see below), so it isn't gone the instant the pointer leaves.
      expect(queryByRole('tooltip')).not.toBeNull();

      await vi.runAllTimersAsync();

      expect(queryByRole('tooltip')).toBeNull();
      expect(trigger.getAttribute('aria-describedby')).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });

  // The regression: mouseleave used to close the tooltip the instant the pointer left
  // the trigger's own layout box, which doesn't cover the gap to the floating content —
  // so moving the pointer toward a link inside the tooltip closed it before arriving.
  // A grace period, cancelled by re-entering (trigger or content) in time, fixes that.
  it('keeps the tooltip open if the pointer returns before the close delay elapses', async () => {
    vi.useFakeTimers();
    try {
      const { wrapper, queryByRole } = setup();

      await fireEvent.mouseEnter(wrapper);
      await fireEvent.mouseLeave(wrapper);
      await vi.advanceTimersByTimeAsync(50); // still mid-gap, well under the delay
      await fireEvent.mouseEnter(wrapper); // pointer arrived at the content

      await vi.runAllTimersAsync();

      expect(queryByRole('tooltip')).not.toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });

  it('dismisses on Escape without moving the pointer (WCAG 2.1 SC 1.4.13)', async () => {
    const { wrapper, trigger, queryByRole } = setup();

    await fireEvent.mouseEnter(wrapper);
    await fireEvent.keyDown(window, { key: 'Escape' });

    expect(queryByRole('tooltip')).toBeNull();
    expect(trigger.getAttribute('aria-describedby')).toBeNull();
  });

  it('ignores other keys', async () => {
    const { wrapper, queryByRole } = setup();

    await fireEvent.mouseEnter(wrapper);
    await fireEvent.keyDown(window, { key: 'a' });

    expect(queryByRole('tooltip')).not.toBeNull();
  });

  it('nests inside the wrapper so the pointer can travel onto it', async () => {
    const { wrapper, queryByRole } = setup();

    await fireEvent.mouseEnter(wrapper);

    // A <div> here would be invalid inside the <span> wrapper.
    const tooltip = queryByRole('tooltip');
    expect(must(tooltip).tagName).toBe('SPAN');
    expect(wrapper.contains(tooltip)).toBe(true);
  });
});
