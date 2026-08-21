import type { Attachment } from 'svelte/attachments';

/**
 * Moves the attached element to `document.body` for as long as it is mounted.
 *
 * A `fixed inset-0` overlay only covers the page when nothing between it and the body
 * has opened a stacking context. `position: sticky` opens one unconditionally — and the
 * job list's filter sidebar is sticky, so a dialog rendered inside it was confined to
 * the sidebar's layer and painted beneath the job cards, which come later in the
 * document. No z-index climbs out of a stacking context; only leaving it does.
 * `transform`, `filter` and `perspective` do the same thing and are just as easy to add
 * to a wrapper by accident.
 *
 * The alternative is to mount every dialog at the view root and thread its open state
 * down (what the All-filters modal does). This keeps a component that owns a dialog
 * owning it, at the cost of one DOM move.
 */
export function portal(): Attachment<HTMLElement> {
  return (node) => {
    document.body.appendChild(node);

    // Detach on teardown, and ONLY detach.
    //
    // The first version put the node back where Svelte had created it, on the theory
    // that Svelte could then remove it from the parent it recorded. That is exactly
    // backwards when the cleanup runs after Svelte's own removal: re-inserting a node
    // Svelte has already taken out RESURRECTS it. The dialog then survived its own
    // close — the backdrop stayed over the page and the panel sat behind the job cards,
    // which reads as "closing is broken" rather than "the portal is".
    //
    // Removing is safe in either order: if Svelte got there first this is a no-op, and
    // if it did not, the node is gone before Svelte looks for it.
    return () => node.remove();
  };
}
