<script lang="ts">
  // Every chat, at one address. The id is optional: `/my/assistant` opens the newest
  // conversation (or starts one) and rewrites the address to that chat's own, while
  // `/my/assistant/<id>` opens the chat it names. The id is in the path rather than a
  // query string so a chat is a real page — bookmarkable, shareable with yourself, and
  // reachable with the browser's back button after switching away from it.
  //
  // Both live in ONE route node on purpose. As two pages, the rewrite from the bare
  // address to a chat's own address swapped one <AssistantChat> for another, and the
  // unmount cancelled whatever turn had just started — which is exactly what an opening
  // message needs to survive.
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { Menu } from '@lucide/svelte';
  import AssistantChat from '#lib/assistant/AssistantChat.svelte';
  import AccountNavRail from '#lib/components/AccountNavRail.svelte';
  import { entryFromQuery, historyModeFor } from '#lib/assistant/presets.js';
  import { messages } from './messages';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { messages as shellMessages } from '#lib/i18n/shell.js';

  const s = $derived(t(messages, locale()));
  const shell = $derived(t(shellMessages, locale()));
  // Below lg the rail folds into a drawer (the same one the tailoring workspace opens), so
  // a phone gets the chat's full width instead of a 56px column of icons beside it.
  let navOpen = $state(false);

  // Where this page starts. It fills the viewport from there down, so the composer at its
  // foot is always on screen. Measured, not assumed: what stands above it is the header
  // AND whichever self-gating bands the root layout shows (the email-verification prompt,
  // whose height changes when its code form opens), and a fixed `100dvh - header` sized
  // the page past the fold by exactly their height. Neither the body nor the page itself
  // can tell when a band changes — html and body are pinned at 100% — so the two boxes
  // observed are the root layout's `min-h-svh` column and its <main>: a band that grows
  // pushes the column past the viewport, and one that shrinks or goes away hands the
  // freed height to <main> (it is `flex-1`, and the column cannot shrink below the
  // viewport). A page that then refits moves neither back out of step, so the measurement
  // settles in one step. Starts at the header's h-14 so the server render matches what it
  // always drew.
  let shellEl = $state<HTMLDivElement | null>(null);
  let top = $state(56);
  $effect(() => {
    const el = shellEl;
    const main = el?.closest('main');
    if (!el || !main) return;
    const measure = () => {
      top = Math.max(0, el.getBoundingClientRect().top + window.scrollY);
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(main);
    if (main.parentElement) observer.observe(main.parentElement);
    return () => observer.disconnect();
  });
  const session = $derived(page.params.id);
  // Read once, not reactively: the query says what this ARRIVAL asked for, and the
  // address is rewritten to the session's own URL — dropping the query — moments later.
  const entry = entryFromQuery(new URLSearchParams(page.url.search));

  function onSessionChange(id: string) {
    const mode = historyModeFor(page.params.id, id);
    if (mode === 'none') return;
    void goto(resolve('/my/assistant/[[id]]', { id }), { replaceState: mode === 'replace' });
  }
</script>

<svelte:head><title>{s.headTitle}</title></svelte:head>

<h1 class="sr-only">{s.heading}</h1>

<div bind:this={shellEl} class="flex" style:height="calc(100dvh - {top}px)">
  <AccountNavRail collapsible bind:open={navOpen} />
  <AssistantChat
    {session}
    preset={entry.preset}
    kickoff={entry.kickoff}
    {onSessionChange}
    showSessionRail
  >
    {#snippet navTrigger()}
      <button
        type="button"
        onclick={() => (navOpen = true)}
        aria-label={shell.rail.menu}
        aria-expanded={navOpen}
        class="flex size-8 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground lg:hidden"
      >
        <Menu class="size-4" aria-hidden="true" />
      </button>
    {/snippet}
  </AssistantChat>
</div>
