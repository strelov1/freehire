import type { Attachment } from 'svelte/attachments';
import { openerOf, restoreFocus } from './focusTrap.js';

/**
 * The item index a key press should move focus to inside a menu, or `null` when the
 * key is not a menu navigation key. Down/Up wrap around; Home/End jump to the ends.
 * From outside the items (`current === -1`, e.g. focus on the menu container), Down
 * enters at the first item and Up at the last. Pure so it can be unit-tested in Node
 * (the DOM glue in `menuNav` is verified via svelte-check + visually).
 */
export function nextMenuIndex(current: number, key: string, count: number): number | null {
  if (count <= 0) return null;
  switch (key) {
    case 'ArrowDown':
      return current === -1 ? 0 : (current + 1) % count;
    case 'ArrowUp':
      return current === -1 ? count - 1 : (current - 1 + count) % count;
    case 'Home':
      return 0;
    case 'End':
      return count - 1;
    default:
      return null;
  }
}

const ITEM = '[role="menuitem"],[role="menuitemcheckbox"],[role="menuitemradio"]';

/**
 * The WAI-ARIA menu button keyboard pattern on a `role="menu"` root, applied with
 * `{@attach menuNav({ onclose })}` to a menu that is rendered only while open.
 *
 * - on mount, focus moves to the first item (if the items render later — a menu
 *   that loads its contents — focus parks on the container and moves to the first
 *   item as soon as one appears);
 * - Down/Up move between items (wrapping), Home/End jump to the ends;
 * - Tab/Shift+Tab close the menu and carry on through the page from the trigger, rather
 *   than walking the items one by one;
 * - on close (the element leaving the DOM — Escape is the host's own handler) focus
 *   returns to whatever was focused when the menu opened, usually the trigger, unless
 *   the app already moved it elsewhere (see `restoreFocus`).
 *
 * Keys pressed inside an inline form in the menu (AddToList's "new list") are left to
 * the form, so arrows and Home/End move the caret and Tab reaches its submit button.
 */
export function menuNav({ onclose }: { onclose: () => void }): Attachment<HTMLElement> {
  return (node) => {
    const opener = openerOf(document.activeElement);

    const items = () =>
      Array.from(node.querySelectorAll<HTMLElement>(ITEM)).filter(
        (el) => !(el as HTMLButtonElement).disabled && el.getClientRects().length > 0,
      );

    let observer: MutationObserver | null = null;
    const first = items()[0];
    if (first) {
      first.focus();
    } else {
      if (!node.hasAttribute('tabindex')) node.setAttribute('tabindex', '-1');
      node.focus();
      observer = new MutationObserver(() => {
        const item = items()[0];
        if (!item) return;
        observer?.disconnect();
        observer = null;
        // Only if the user has not moved on in the meantime.
        if (document.activeElement === node) item.focus();
      });
      observer.observe(node, { childList: true, subtree: true });
    }

    const onKeydown = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement | null;
      // An inline form in the menu (AddToList's "new list") keeps its own keys: arrows
      // move the caret and Tab walks from the field to its submit button.
      if (target?.closest('input, textarea, form')) return;
      if (e.key === 'Tab') {
        // Close, and let the browser's own Tab move on. Focus goes back to the opener
        // first, so the next stop is the one after (or before) the trigger, not the
        // next item of a menu that is about to disappear.
        opener?.focus();
        onclose();
        return;
      }
      const list = items();
      const next = nextMenuIndex(list.indexOf(document.activeElement as HTMLElement), e.key, list.length);
      if (next === null) return;
      e.preventDefault();
      list[next]?.focus();
    };

    node.addEventListener('keydown', onKeydown);

    return () => {
      observer?.disconnect();
      node.removeEventListener('keydown', onKeydown);
      restoreFocus(opener, node);
    };
  };
}
