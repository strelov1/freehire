import { fireEvent, render } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { describe, expect, it } from 'vitest';
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
  it('drops the description again when it closes', async () => {
    const { wrapper, trigger, queryByRole } = setup();

    await fireEvent.mouseEnter(wrapper);
    await fireEvent.mouseLeave(wrapper);

    expect(queryByRole('tooltip')).toBeNull();
    expect(trigger.getAttribute('aria-describedby')).toBeNull();
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
