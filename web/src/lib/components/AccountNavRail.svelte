<script lang="ts">
  import { MediaQuery } from 'svelte/reactivity';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { toPath } from '#lib/pathname.js';
  import { X } from '@lucide/svelte';
  import { isAuthenticated, currentUser } from '#lib/auth.svelte.js';
  import { visibleAccountNav, isSectionActive } from '#lib/accountNav.js';
  import { accountNavIcons } from '#lib/accountNavIcons.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { messages, navLabel } from '#lib/i18n/shell.js';
  import { t } from '#lib/i18n/t.js';
  import { cn, lockScroll, unlockScroll } from '#lib/ui/index.js';
  import { focusTrap } from '#lib/actions/focusTrap.js';

  // A compact, icon-only mirror of the account-section navigation (see
  // my/+layout.svelte), pinned to the left edge of full-width surfaces that reset
  // past the account shell. Same items, gating, order, and active rule as the
  // sidebar; reuses the shared visible-nav model and icon map so there's one source
  // of truth. Renders nothing for a signed-out visitor.
  //
  // `collapsible` opts a host into responsive behaviour: at lg+ the icon rail shows
  // as usual, but below lg it disappears (freeing the width) and instead opens as a
  // labelled slide-in drawer driven by `open` — the host renders its own trigger
  // (e.g. a burger in its header) and binds `open`. Without `collapsible` the rail
  // renders exactly as before at every width.

  let { open = $bindable(false), collapsible = false }: { open?: boolean; collapsible?: boolean } =
    $props();
  const uid = $props.id();
  const drawerTitleId = `${uid}-drawer-title`;

  const s = $derived(t(messages, locale()));
  const path = $derived(page.url.pathname);
  const navItems = $derived(
    visibleAccountNav(currentUser()?.role === 'moderator', currentUser()?.beta_tester ?? false),
  );

  const close = () => (open = false);
  const onKey = (e: KeyboardEvent) => {
    if (open && e.key === 'Escape') close();
  };

  // The drawer exists only below lg. Widening the window past it hides the drawer
  // without closing it, which would leave the page locked behind nothing; close it.
  const wide = new MediaQuery('min-width: 1024px', false);
  $effect(() => {
    if (wide.current) open = false;
  });

  // The mobile drawer is modal: hold the shared scroll lock while it is open.
  $effect(() => {
    if (!collapsible || !open) return;
    lockScroll();
    return () => unlockScroll();
  });
</script>

{#snippet navLink(item: ReturnType<typeof visibleAccountNav>[number], withLabel: boolean)}
  {@const active = isSectionActive(path, item.href)}
  {@const Icon = accountNavIcons[item.href]}
  {@const label = navLabel(s, item.href, item.label)}
  <a
    href={resolve(toPath(item.href))}
    aria-current={active ? 'page' : undefined}
    title={withLabel ? undefined : label}
    onclick={withLabel ? close : undefined}
    class={cn(
      'flex items-center rounded-md transition-colors',
      withLabel ? 'gap-3 px-3 py-2' : 'justify-center p-2',
      active
        ? 'bg-secondary text-secondary-foreground'
        : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
    )}
  >
    <Icon class="size-4 shrink-0" />
    {#if withLabel}
      <span class="text-sm">{label}</span>
    {:else}
      <span class="sr-only">{label}</span>
    {/if}
  </a>
{/snippet}

<svelte:window onkeydown={onKey} />

{#if isAuthenticated()}
  {#if collapsible}
    <!-- Desktop icon rail (lg+): identical to the non-collapsible rail. -->
    <nav
      aria-label={s.shell.accountSections}
      class="hidden w-14 shrink-0 flex-col gap-1 overflow-y-auto border-r border-border bg-background p-2 lg:flex"
    >
      {#each navItems as item (item.href)}
        {@render navLink(item, false)}
      {/each}
    </nav>

    <!-- Mobile drawer (<lg): a labelled slide-in over a dimmed backdrop. -->
    {#if open}
      <button
        type="button"
        aria-label={s.rail.closeMenu}
        onclick={close}
        class="fixed inset-0 z-40 bg-black/40 lg:hidden"
      ></button>
      <!-- A modal dialog: focusTrap moves focus in, keeps Tab inside, and returns it
           to the host's trigger when the drawer closes. -->
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={drawerTitleId}
        class="fixed left-0 top-0 z-50 flex h-full w-64 flex-col gap-1 overflow-y-auto border-r border-border bg-background p-2 lg:hidden"
        {@attach focusTrap()}
      >
        <div class="mb-1 flex items-center justify-between px-2 py-1.5">
          <span id={drawerTitleId} class="text-sm font-semibold text-foreground"
            >{s.rail.menu}</span
          >
          <button
            type="button"
            aria-label={s.rail.closeMenu}
            onclick={close}
            class="rounded-md p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
          >
            <X class="size-4" />
          </button>
        </div>
        <nav aria-label={s.shell.accountSections} class="flex flex-col gap-1">
          {#each navItems as item (item.href)}
            {@render navLink(item, true)}
          {/each}
        </nav>
      </div>
    {/if}
  {:else}
    <nav
      aria-label={s.shell.accountSections}
      class="flex w-14 shrink-0 flex-col gap-1 overflow-y-auto border-r border-border bg-background p-2"
    >
      {#each navItems as item (item.href)}
        {@render navLink(item, false)}
      {/each}
    </nav>
  {/if}
{/if}
