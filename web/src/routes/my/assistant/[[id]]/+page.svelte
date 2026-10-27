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
  import { fillViewport } from '#lib/actions/fillViewport.js';
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

<div use:fillViewport class="flex" style:height="calc(100dvh - 3.5rem)">
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
