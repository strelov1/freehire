<script lang="ts">
  /**
   * An assistant conversation opened beside the page it is about, instead of in place of it.
   *
   * Built for the experience interviewer, where navigating to the assistant threw away half
   * of the subject: the selection was gone and the near-duplicate pair they had just spotted
   * was off screen. The application chat has the same shape — the conversation is about the
   * job drawer still on screen. Docked, both halves stay on one page — which is also why
   * this panel is deliberately NOT modal where there is room for two columns. A focus trap
   * and an inert background exist to stop someone touching what is behind, and what is
   * behind is the whole point.
   *
   * The host supplies the chat itself as `children`, which is handed the setter the header's
   * "Open full" link reads.
   */
  import type { Snippet } from 'svelte';
  import { ExternalLink, X } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { lockScroll, unlockScroll } from '#lib/ui/index.js';
  import { focusTrap } from '#lib/actions/focusTrap.js';
  import { DOCK_WIDTH, DOCKED_QUERY, setDockOffset } from '#lib/assistantDock.svelte.js';

  let {
    open = false,
    title,
    onClose,
    children,
  }: {
    open?: boolean;
    /** The panel's heading, also its accessible name. */
    title: string;
    onClose: () => void;
    /** The conversation, given the setter for the session id the "Open full" link needs. */
    children: Snippet<[(id: string) => void]>;
  } = $props();

  // Whether there is room to dock beside the bank rather than cover it. The threshold and
  // its arithmetic live with the offset, since they are the same decision.
  let docked = $state(true);

  $effect(() => {
    const mq = window.matchMedia(DOCKED_QUERY);
    const sync = () => (docked = mq.matches);
    sync();
    mq.addEventListener('change', sync);
    return () => mq.removeEventListener('change', sync);
  });

  // Tell the shell how much room to give up. A covering overlay takes none — it is drawn
  // over the page, and pushing the page aside underneath it would be movement nobody can
  // see and everybody's scroll position would pay for. Released on unmount so leaving the
  // Experience tab with the panel open does not leave the rest of the account indented.
  $effect(() => {
    setDockOffset(open && docked ? DOCK_WIDTH : 0);
    return () => setDockOffset(0);
  });

  // Only the covering form locks the page — the docked one is a column in the flow, and
  // locking there would freeze the bank this panel exists to keep usable.
  $effect(() => {
    if (!open || docked) return;
    lockScroll();
    return () => unlockScroll();
  });

  // Mounting the chat boots a session, so it must not happen until the candidate has
  // actually asked for one: mounting it eagerly would mint an empty conversation for
  // everyone who ever opened the page. After the first open it STAYS mounted
  // and is only hidden, because unmounting cancels a turn that is still streaming.
  let everOpened = $state(false);
  $effect(() => {
    if (open) everOpened = true;
  });

  // The panel shows no session rail, so this is the only way back to the candidate's other
  // conversations — and the way to keep this one when it outgrows a 360px column.
  let sessionId = $state<string | null>(null);
</script>

<svelte:window
  onkeydown={(e) => {
    // Escape closes the covering form only. Docked, it is a column beside the page and
    // stealing Escape from whatever the candidate is editing in the bank would be wrong.
    if (e.key === 'Escape' && open && !docked) onClose();
  }}
/>

{#if everOpened}
  <!-- The outer element carries position and visibility ONLY: `hidden` and a `flex`
       utility on one element is a display collision decided by stylesheet order. -->
  <div
    class={[
      // Docked, this is pinned to the VIEWPORT rather than laid out in the tab body. It
      // used to be a column inside the account content area, which meant it spent the
      // bank's width — the conversation and the achievements it is about were both cramped.
      // Below the header, like the selection action bar, and under it in the stack.
      // Covering, it fades in each time it is shown: un-hiding restarts the animation.
      docked ? 'fixed bottom-0 right-0 top-14 z-30' : 'fixed inset-0 z-50 animate-modal-fade',
      !open && 'hidden',
    ]}
    style:width={docked ? `${DOCK_WIDTH}px` : undefined}
    role={docked ? undefined : 'dialog'}
    aria-modal={docked ? undefined : 'true'}
    aria-label={title}
    {@attach open && !docked ? focusTrap() : undefined}
  >
    <div
      class={[
        'flex h-full flex-col overflow-hidden bg-card',
        // Flush against the viewport edge now, so it is a wall rather than a card: one
        // border on the side that meets the page, and no rounding on edges that touch
        // nothing.
        docked && 'border-l border-border',
        !docked && 'bg-background',
      ]}
    >
      <header class="flex shrink-0 items-center gap-2 border-b border-border px-3 py-2">
        <h2 class="min-w-0 flex-1 truncate text-sm font-semibold">{title}</h2>
        {#if sessionId}
          <a
            href={resolve('/my/assistant/[[id]]', { id: sessionId })}
            class="inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
            title="Open this conversation full width"
          >
            <ExternalLink class="size-3.5" />
            Open full
          </a>
        {/if}
        <button
          type="button"
          onclick={onClose}
          aria-label="Close assistant"
          class="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
        >
          <X class="size-4" />
        </button>
      </header>

      <div class="flex min-h-0 flex-1">
        {@render children((id) => (sessionId = id))}
      </div>
    </div>
  </div>
{/if}
