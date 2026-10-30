<script lang="ts">
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { messages } from '#lib/i18n/shell.js';
  import { t } from '#lib/i18n/t.js';
  import AccountNavDrawer from './AccountNavDrawer.svelte';
  import AccountNavGroups from './AccountNavGroups.svelte';

  // A compact, icon-only mirror of the account-section navigation (see
  // my/+layout.svelte), pinned to the left edge of full-width surfaces that reset
  // past the account shell. Same items, gating, order, and active rule as the
  // sidebar; renders through the same AccountNavGroups so there's one source of
  // truth. Renders nothing for a signed-out visitor.
  //
  // `collapsible` opts a host into responsive behaviour: at lg+ the icon rail shows
  // as usual, but below lg it disappears (freeing the width) and instead opens as a
  // labelled slide-in drawer driven by `open` — the host renders its own trigger
  // (e.g. a burger in its header) and binds `open`. Without `collapsible` the rail
  // renders exactly as before at every width.

  let { open = $bindable(false), collapsible = false }: { open?: boolean; collapsible?: boolean } =
    $props();
  const s = $derived(t(messages, locale()));
</script>

{#if isAuthenticated()}
  {#if collapsible}
    <!-- Desktop icon rail (lg+): identical to the non-collapsible rail. -->
    <nav
      aria-label={s.shell.accountSections}
      class="hidden w-14 shrink-0 flex-col gap-1 overflow-y-auto border-r border-border bg-background p-2 lg:flex"
    >
      <AccountNavGroups variant="rail" />
    </nav>

    <!-- Mobile drawer (<lg): a labelled slide-in over a dimmed backdrop. -->
    <AccountNavDrawer bind:open />
  {:else}
    <nav
      aria-label={s.shell.accountSections}
      class="flex w-14 shrink-0 flex-col gap-1 overflow-y-auto border-r border-border bg-background p-2"
    >
      <AccountNavGroups variant="rail" />
    </nav>
  {/if}
{/if}
