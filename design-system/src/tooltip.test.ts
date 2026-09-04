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
  return { container, wrapper: must(trigger.parentElement), trigger, queryByRole };
}

// Every hover interaction now runs on a timer (the show delay below), so these tests
// drive fake ones instead of waiting on real milliseconds.
async function withFakeTimers(run: () => Promise<void>) {
  vi.useFakeTimers();
  try {
    await run();
  } finally {
    vi.useRealTimers();
  }
}

describe('Tooltip', () => {
  it('shows nothing until the trigger is hovered or focused', () => {
    const { trigger, queryByRole } = setup();

    expect(queryByRole('tooltip')).toBeNull();
    expect(trigger.getAttribute('aria-describedby')).toBeNull();
  });

  // The regression: role="tooltip" associates nothing on its own, so without
  // this link the tooltip was invisible to a screen reader.
  it('describes the trigger while it is open', () =>
    withFakeTimers(async () => {
      const { wrapper, trigger, queryByRole } = setup();

      await fireEvent.mouseEnter(wrapper);
      await vi.runAllTimersAsync(); // past the show delay below

      const tooltip = must(queryByRole('tooltip'));
      expect(tooltip.id).toBeTruthy();
      expect(trigger.getAttribute('aria-describedby')).toBe(tooltip.id);
    }));

  // Hovering across a row of chips fires mouseenter/mouseleave on each one a gliding
  // pointer passes over — without a delay every chip along the way would flash its
  // tooltip open before the pointer even settles on the one actually being read.
  it('does not show on hover until the pointer has sat still for the show delay', () =>
    withFakeTimers(async () => {
      const { wrapper, queryByRole } = setup();

      await fireEvent.mouseEnter(wrapper);

      expect(queryByRole('tooltip')).toBeNull();
    }));

  // The throttling guarantee itself: a pointer gliding past never holds still long
  // enough to arm the show timer, so leaving before the delay elapses must cancel it
  // outright rather than merely deferring the open.
  it('cancels a pending show if the pointer leaves before the show delay elapses', () =>
    withFakeTimers(async () => {
      const { wrapper, queryByRole } = setup();

      await fireEvent.mouseEnter(wrapper);
      await fireEvent.mouseLeave(wrapper);
      await vi.runAllTimersAsync();

      expect(queryByRole('tooltip')).toBeNull();
    }));

  // Focus is a single deliberate action (unlike a hover glide), so — unlike the tests
  // above — this one needs no fake timers at all: with a real, unadvanced clock, the
  // tooltip is already open, which is only possible if focus carries no show delay.
  it('opens on focus too, not only on hover', async () => {
    const { wrapper, trigger, queryByRole } = setup();

    await fireEvent.focusIn(wrapper);

    expect(queryByRole('tooltip')).not.toBeNull();
    expect(trigger.getAttribute('aria-describedby')).toBeTruthy();
  });

  // Leaving the attribute behind would point at an id that no longer exists,
  // which reads as no description at all.
  it('drops the description again once the close delay elapses', () =>
    withFakeTimers(async () => {
      const { wrapper, trigger, queryByRole } = setup();

      await fireEvent.mouseEnter(wrapper);
      await vi.runAllTimersAsync(); // past the show delay
      await fireEvent.mouseLeave(wrapper);
      // The hide is deferred (see below), so it isn't gone the instant the pointer leaves.
      expect(queryByRole('tooltip')).not.toBeNull();

      await vi.runAllTimersAsync();

      expect(queryByRole('tooltip')).toBeNull();
      expect(trigger.getAttribute('aria-describedby')).toBeNull();
    }));

  // The regression: mouseleave used to close the tooltip the instant the pointer left
  // the trigger's own layout box, which doesn't cover the gap to the floating content —
  // so moving the pointer toward a link inside the tooltip closed it before arriving.
  // A grace period, cancelled by re-entering (trigger or content) in time, fixes that.
  it('keeps the tooltip open if the pointer returns before the close delay elapses', () =>
    withFakeTimers(async () => {
      const { wrapper, queryByRole } = setup();

      await fireEvent.mouseEnter(wrapper);
      await vi.runAllTimersAsync(); // past the show delay
      await fireEvent.mouseLeave(wrapper);
      await vi.advanceTimersByTimeAsync(50); // still mid-gap, well under the delay
      await fireEvent.mouseEnter(wrapper); // pointer arrived at the content

      await vi.runAllTimersAsync();

      expect(queryByRole('tooltip')).not.toBeNull();
    }));

  it('dismisses on Escape without moving the pointer (WCAG 2.1 SC 1.4.13)', () =>
    withFakeTimers(async () => {
      const { wrapper, trigger, queryByRole } = setup();

      await fireEvent.mouseEnter(wrapper);
      await vi.runAllTimersAsync(); // past the show delay
      expect(queryByRole('tooltip')).not.toBeNull(); // otherwise Escape has nothing to dismiss

      await fireEvent.keyDown(window, { key: 'Escape' });

      expect(queryByRole('tooltip')).toBeNull();
      expect(trigger.getAttribute('aria-describedby')).toBeNull();
    }));

  it('ignores other keys', () =>
    withFakeTimers(async () => {
      const { wrapper, queryByRole } = setup();

      await fireEvent.mouseEnter(wrapper);
      await vi.runAllTimersAsync(); // past the show delay
      await fireEvent.keyDown(window, { key: 'a' });

      expect(queryByRole('tooltip')).not.toBeNull();
    }));

  // A touch pointer has no hover, so before this the tooltip was simply unreachable on
  // a phone — the one place a reader is most likely to meet a skill chip.
  describe('touch', () => {
    const tap = (el: Element) => fireEvent.pointerDown(el, { pointerType: 'touch' });

    it('opens on tap', async () => {
      const { wrapper, trigger, queryByRole } = setup();

      await tap(wrapper);

      expect(queryByRole('tooltip')).not.toBeNull();
      expect(trigger.getAttribute('aria-describedby')).toBeTruthy();
    });

    it('closes on a second tap of the trigger', async () => {
      const { wrapper, queryByRole } = setup();

      await tap(wrapper);
      await tap(wrapper);

      expect(queryByRole('tooltip')).toBeNull();
    });

    it('closes on a tap anywhere else', async () => {
      const { wrapper, queryByRole } = setup();

      await tap(wrapper);
      await tap(document.body);

      expect(queryByRole('tooltip')).toBeNull();
    });

    // The tap that opened it must not also be the tap that dismisses it.
    it('stays open after the tap that opened it', async () => {
      const { wrapper, queryByRole } = setup();

      await tap(wrapper);

      expect(queryByRole('tooltip')).not.toBeNull();
    });

    // The content is a descendant of the wrapper the toggle listens on, so a tap on a
    // link inside the tooltip would close it — unmounting the link before the click
    // that follows the pointerdown could land on it. The navigation would silently do
    // nothing, on the one pointer type this whole path exists for.
    it('stays open when the tap lands inside the tooltip’s own content', async () => {
      const { wrapper, queryByRole } = setup();

      await tap(wrapper);
      const tooltip = must(queryByRole('tooltip'));
      await tap(tooltip);

      expect(queryByRole('tooltip')).not.toBeNull();
    });

    // A mouse already has hover. Toggling on its pointerdown too would close the
    // tooltip the moment someone clicked a link inside it.
    it('ignores a mouse pointerdown', () =>
      withFakeTimers(async () => {
        const { wrapper, queryByRole } = setup();

        await fireEvent.mouseEnter(wrapper);
        await vi.runAllTimersAsync(); // past the show delay
        await fireEvent.pointerDown(wrapper, { pointerType: 'mouse' });

        expect(queryByRole('tooltip')).not.toBeNull();
      }));

    // A pen hovers, so hover already opened the tooltip — and then pressing the trigger
    // would toggle it shut under the very pointer that is aiming at it.
    it('ignores a pen pointerdown, because a pen hovers', () =>
      withFakeTimers(async () => {
        const { wrapper, queryByRole } = setup();

        await fireEvent.mouseEnter(wrapper);
        await vi.runAllTimersAsync(); // past the show delay
        await fireEvent.pointerDown(wrapper, { pointerType: 'pen' });

        expect(queryByRole('tooltip')).not.toBeNull();
      }));
  });

  // The content is next in DOM order, so Tab out of the trigger lands on the link
  // inside it. A bare focusout hid the tooltip first, removing the link mid-focus —
  // a keyboard reader could read the description and never reach where it points.
  it('stays open while focus moves into its own content', async () => {
    const { wrapper, queryByRole } = setup();

    await fireEvent.focusIn(wrapper);
    const tooltip = must(queryByRole('tooltip'));
    await fireEvent.focusOut(wrapper, { relatedTarget: tooltip });

    expect(queryByRole('tooltip')).not.toBeNull();
  });

  it('closes when focus leaves for something outside it', async () => {
    const { wrapper, queryByRole } = setup();

    await fireEvent.focusIn(wrapper);
    await fireEvent.focusOut(wrapper, { relatedTarget: document.body });

    expect(queryByRole('tooltip')).toBeNull();
  });

  it('nests inside the wrapper so the pointer can travel onto it', () =>
    withFakeTimers(async () => {
      const { wrapper, queryByRole } = setup();

      await fireEvent.mouseEnter(wrapper);
      await vi.runAllTimersAsync(); // past the show delay

      // A <div> here would be invalid inside the <span> wrapper.
      const tooltip = queryByRole('tooltip');
      expect(must(tooltip).tagName).toBe('SPAN');
      expect(wrapper.contains(tooltip)).toBe(true);
    }));

  it('shifts the content back inside the viewport when centering it would overflow', () =>
    withFakeTimers(async () => {
      const originalRect = Element.prototype.getBoundingClientRect;
      const originalWidth = window.innerWidth;
      try {
        Element.prototype.getBoundingClientRect = () =>
          ({ left: 280, right: 480, top: 0, bottom: 20, width: 200, height: 20, x: 280, y: 0, toJSON() {} }) as DOMRect;
        Object.defineProperty(window, 'innerWidth', { value: 400, configurable: true });

        const { wrapper, queryByRole } = setup();

        await fireEvent.mouseEnter(wrapper);
        await vi.runAllTimersAsync(); // past the show delay

        const tooltip = must(queryByRole('tooltip'));
        // Centered, the mocked box's right edge (480) sits 88px past the 392px boundary
        // (innerWidth 400 minus the 8px edge padding) — the shift must cancel exactly that.
        expect(tooltip.style.transform).toContain('-88px');
      } finally {
        Element.prototype.getBoundingClientRect = originalRect;
        Object.defineProperty(window, 'innerWidth', { value: originalWidth, configurable: true });
      }
    }));

  // Regression guard: a box wider than the viewport itself overflows both edges at
  // once. A naive "correct whichever edge overflows" shift would push the near edge
  // out even further while fixing the far one — this asserts the near (left) edge
  // stays flush against the padding instead, even though the far edge still overflows.
  it('keeps the near edge visible when the content is wider than the viewport, rather than pushing it out further', () =>
    withFakeTimers(async () => {
      const originalRect = Element.prototype.getBoundingClientRect;
      const originalWidth = window.innerWidth;
      try {
        // Centered box: left -10, right 310 — 320px wide, wider than the 284px of
        // usable width an 8px-padded 300px viewport leaves.
        Element.prototype.getBoundingClientRect = () =>
          ({ left: -10, right: 310, top: 0, bottom: 20, width: 320, height: 20, x: -10, y: 0, toJSON() {} }) as DOMRect;
        Object.defineProperty(window, 'innerWidth', { value: 300, configurable: true });

        const { wrapper, queryByRole } = setup();

        await fireEvent.mouseEnter(wrapper);
        await vi.runAllTimersAsync(); // past the show delay

        const tooltip = must(queryByRole('tooltip'));
        // Shifting right by 18px puts the near edge exactly at the 8px padding
        // (-10 + 18 = 8). Shifting left instead (the naive far-edge correction) would
        // have moved the near edge to -28 — further off screen, not less.
        expect(tooltip.style.transform).toContain('18px');
        expect(tooltip.style.transform).not.toContain('-18px');
      } finally {
        Element.prototype.getBoundingClientRect = originalRect;
        Object.defineProperty(window, 'innerWidth', { value: originalWidth, configurable: true });
      }
    }));
});
