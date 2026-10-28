<script lang="ts">
  import { MediaQuery } from 'svelte/reactivity';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { toPath } from '#lib/pathname.js';
  import { X } from '@lucide/svelte';
  import { currentUser } from '#lib/auth.svelte.js';
  import { visibleAccountNav, isSectionActive } from '#lib/accountNav.js';
  import { accountNavIcons } from '#lib/accountNavIcons.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { messages, navLabel } from '#lib/i18n/shell.js';
  import { t } from '#lib/i18n/t.js';
  import { cn, lockScroll, unlockScroll } from '#lib/ui/index.js';
  import { focusTrap } from '#lib/actions/focusTrap.js';

  // The account-section navigation as a labelled slide-in drawer, for widths below lg.
  // The host renders its own trigger (a burger, a section switcher) and binds `open`.
  // Same items, gating, order, and active rule as the sidebar and the icon rail.

  let { open = $bindable(false) }: { open?: boolean } = $props();
  const uid = $props.id();
  const titleId = `${uid}-drawer-title`;

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

  // The drawer is modal: hold the shared scroll lock while it is open.
  $effect(() => {
    if (!open) return;
    lockScroll();
    return () => unlockScroll();
  });
</script>

<svelte:window onkeydown={onKey} />

{#if open}
  <button
    type="button"
    aria-label={s.rail.closeMenu}
    onclick={close}
    class="fixed inset-0 z-40 animate-modal-fade bg-black/40 lg:hidden"
  ></button>
  <!-- A modal dialog: focusTrap moves focus in, keeps Tab inside, and returns it
       to the host's trigger when the drawer closes. -->
  <div
    role="dialog"
    aria-modal="true"
    aria-labelledby={titleId}
    class="fixed left-0 top-0 z-50 flex h-full w-64 animate-sheet-in-left flex-col gap-1 overflow-y-auto border-r border-border bg-background p-2 lg:hidden"
    {@attach focusTrap()}
    {@attach (el) => {
      // Twenty sections outgrow a short phone, and the drawer opens at its top: bring the
      // section you are in into view, so the menu's first frame says where you are.
      el.querySelector('[aria-current="page"]')?.scrollIntoView({ block: 'nearest' });
    }}
  >
    <div class="mb-1 flex items-center justify-between px-2 py-1.5">
      <span id={titleId} class="text-sm font-semibold text-foreground">{s.rail.menu}</span>
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
        {@const active = isSectionActive(path, item.href)}
        {@const Icon = accountNavIcons[item.href]}
        <a
          href={resolve(toPath(item.href))}
          aria-current={active ? 'page' : undefined}
          onclick={close}
          class={cn(
            'flex items-center gap-3 rounded-md px-3 py-2 transition-colors',
            active
              ? 'bg-secondary text-secondary-foreground'
              : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
          )}
        >
          <Icon class="size-4 shrink-0" />
          <span class="text-sm">{navLabel(s, item.href, item.label)}</span>
        </a>
      {/each}
    </nav>
  </div>
{/if}
