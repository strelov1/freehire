import type { Attachment } from 'svelte/attachments';

/**
 * Reports whether `node` has an ancestor that traps a fixed overlay — an element whose
 * style makes it a stacking context, so a `z-50` child cannot paint above anything
 * outside it however high the number goes.
 *
 * Pure so the rule is unit-testable in Node; the DOM glue lives in `portal` below.
 *
 * The properties listed are the ones that bite in this app's layouts. `position: sticky`
 * is the one that actually did: the job list's filter sidebar is sticky, so a dialog
 * rendered inside it painted *under* the job cards, which come later in the document.
 * `transform`, `filter` and `perspective` are here because they do the same thing and
 * are just as easy to add to a wrapper by accident.
 */
export function trapsOverlays(styleOf: (el: Element) => Partial<CSSStyleDeclaration>, node: Element): boolean {
  for (let el = node.parentElement; el; el = el.parentElement) {
    const style = styleOf(el);
    if (style.position === 'sticky' || style.position === 'fixed') return true;
    for (const value of [style.transform, style.filter, style.perspective]) {
      if (value && value !== 'none') return true;
    }
  }
  return false;
}

/**
 * Moves the attached element to `document.body` for as long as it is mounted.
 *
 * A `fixed inset-0` overlay only covers the page when nothing between it and the body
 * has opened a stacking context. `position: sticky` opens one unconditionally, so a
 * dialog rendered inside a sticky sidebar is confined to the sidebar's layer and paints
 * beneath every sibling that comes later in the document — which is what put the job
 * cards on top of the AI filter dialog.
 *
 * The alternative is to mount every dialog at the view root and thread its open state
 * down (what the All-filters modal does). This keeps a component that owns a dialog
 * owning it, at the cost of one DOM move.
 */
export function portal(): Attachment<HTMLElement> {
  return (node) => {
    const parent = node.parentNode;
    const anchor = node.nextSibling;
    document.body.appendChild(node);

    // Put it back where Svelte expects to find it. Svelte removes the node itself on
    // destroy, and it can only do that from the parent it recorded.
    return () => {
      if (parent?.isConnected) parent.insertBefore(node, anchor);
      else node.remove();
    };
  };
}
