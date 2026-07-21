import type { Attachment } from 'svelte/attachments';

/**
 * The focusable index a Tab press should move to inside a focus trap, or `null`
 * when the browser's own tabbing already keeps focus in-bounds. Pure so the wrap
 * logic is unit-testable in Node (the DOM glue in `focusTrap` is verified via
 * svelte-check + visually).
 *
 * - focus escaped the trap (`current === -1`) → pull it to the first item
 * - forward Tab on the last item → wrap to the first
 * - backward Shift+Tab on the first item → wrap to the last
 * - anything else → `null`, let the browser move focus normally
 */
export function nextTrapIndex(current: number, count: number, shift: boolean): number | null {
  if (count <= 0) return null;
  if (current === -1) return 0;
  if (!shift && current === count - 1) return 0;
  if (shift && current === 0) return count - 1;
  return null;
}

const FOCUSABLE =
  'a[href],button:not([disabled]),input:not([disabled]),select:not([disabled]),textarea:not([disabled]),[tabindex]:not([tabindex="-1"])';

/**
 * A modal focus trap for a `role="dialog"` root, applied with `{@attach focusTrap()}`.
 * On mount it moves focus to the dialog container (making it programmatically
 * focusable) so screen readers announce the dialog and Tab starts inside it; while
 * open it keeps Tab/Shift+Tab within the dialog; on close (the element leaving the
 * DOM) it restores focus to whatever was focused when the dialog opened — usually
 * the trigger. This backs the `aria-modal="true"` promise the markup makes.
 */
export function focusTrap(): Attachment<HTMLElement> {
  return (node) => {
    const trigger = document.activeElement as HTMLElement | null;

    const focusables = () =>
      Array.from(node.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(
        (el) => el.getClientRects().length > 0,
      );

    // Focus the container itself (not a specific control), so we neither steal focus
    // to an arbitrary button nor fight any inner autofocus. Needs a tabindex to be
    // programmatically focusable.
    if (!node.hasAttribute('tabindex')) node.setAttribute('tabindex', '-1');
    node.focus();

    const onKeydown = (e: KeyboardEvent) => {
      if (e.key !== 'Tab') return;
      const items = focusables();
      const current = items.indexOf(document.activeElement as HTMLElement);
      const next = nextTrapIndex(current, items.length, e.shiftKey);
      if (next === null) return;
      e.preventDefault();
      items[next]?.focus();
    };

    node.addEventListener('keydown', onKeydown);

    return () => {
      node.removeEventListener('keydown', onKeydown);
      // Restore focus to the opener, but only if focus is still inside the dialog —
      // if the app already moved it elsewhere (e.g. a navigation), leave it be.
      if (node.contains(document.activeElement)) trigger?.focus?.();
    };
  };
}
