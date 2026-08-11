<script lang="ts">
  /**
   * The experience interviewer, opened beside the bank instead of in place of it.
   *
   * The conversation is about the rows the candidate is looking at, so navigating to the
   * assistant to have it threw away half of the subject: the selection was gone and the
   * near-duplicate pair they had just spotted was off screen. Docked, both halves stay on
   * one page — which is also why this panel is deliberately NOT modal where there is room
   * for two columns. A focus trap and an inert background exist to stop someone touching
   * what is behind, and what is behind is the whole point.
   */
  import { ExternalLink, X } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import AssistantChat from '$lib/assistant/AssistantChat.svelte';
  import { lockScroll, unlockScroll } from '$lib/scrollLock';
  import { DOCK_WIDTH, DOCKED_QUERY, setDockOffset } from '$lib/assistantDock.svelte';

  let {
    open = false,
    launch,
    onClose,
    onBankChanged,
    onTurnStateChange = undefined,
  }: {
    open?: boolean;
    /**
     * Which conversation to hold, and what to open it with. The id is a token, not an
     * identifier the server knows: bumping it remounts the chat, which is the only way to
     * aim it at a new subject — `AssistantChat` reads `kickoff` once at construction into a
     * deliberately non-reactive `arrival` and spends it after the first send, so setting the
     * prop on a live chat does nothing at all.
     */
    launch: { id: number; kickoff: string };
    onClose: () => void;
    /** A turn finished; the bank behind this panel may no longer match what it shows. */
    onBankChanged: () => void;
    onTurnStateChange?: (active: boolean) => void;
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
  // actually asked for one: mounting it eagerly would mint an empty `profile` conversation
  // for everyone who ever opened the Experience tab. After the first open it STAYS mounted
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
      docked ? 'fixed bottom-0 left-0 top-14 z-30' : 'fixed inset-0 z-50',
      !open && 'hidden',
    ]}
    style:width={docked ? `${DOCK_WIDTH}px` : undefined}
    role={docked ? undefined : 'dialog'}
    aria-modal={docked ? undefined : 'true'}
    aria-label="Experience assistant"
  >
    <div
      class={[
        'flex h-full flex-col overflow-hidden bg-card',
        // Flush against the viewport edge now, so it is a wall rather than a card: one
        // border on the side that meets the page, and no rounding on edges that touch
        // nothing.
        docked && 'border-r border-border',
        !docked && 'bg-background',
      ]}
    >
      <header class="flex shrink-0 items-center gap-2 border-b border-border px-3 py-2">
        <h2 class="min-w-0 flex-1 truncate text-sm font-semibold">Experience assistant</h2>
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
        {#key launch.id}
          <AssistantChat
            preset="profile"
            kickoff={launch.kickoff}
            showSessionRail={false}
            sessionLabel="Experience"
            onSessionChange={(id) => (sessionId = id)}
            onTurnComplete={onBankChanged}
            {onTurnStateChange}
          />
        {/key}
      </div>
    </div>
  </div>
{/if}
