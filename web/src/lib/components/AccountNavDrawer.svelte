<script lang="ts">
  import { MediaQuery } from 'svelte/reactivity';
  import { X } from '@lucide/svelte';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { messages } from '#lib/i18n/shell.js';
  import { t } from '#lib/i18n/t.js';
  import { lockScroll, unlockScroll } from '#lib/ui/index.js';
  import AccountNavGroups from './AccountNavGroups.svelte';
  import { focusTrap } from '#lib/actions/focusTrap.js';

  // The account-section navigation as a labelled slide-in drawer, for widths below lg.
  // The host renders its own trigger (a burger, a section switcher) and binds `open`.
  // Same workspace, groups, gating and active rule as the sidebar and the icon rail.

  let { open = $bindable(false) }: { open?: boolean } = $props();
  const uid = $props.id();
  const titleId = `${uid}-drawer-title`;

  const s = $derived(t(messages, locale()));

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
       to the host's trigger when the drawer closes. The page opts into
       `viewport-fit=cover`, so a full-height sheet pinned to the left edge pads itself
       clear of the home indicator below and, in landscape, the notch beside it. -->
  <div
    role="dialog"
    aria-modal="true"
    aria-labelledby={titleId}
    class="fixed left-0 top-0 z-50 flex h-full w-64 animate-sheet-in-left flex-col gap-1 overflow-y-auto border-r border-border bg-background sheet-left-safe lg:hidden"
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
        class="-mr-1 flex size-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground any-pointer-coarse:size-11"
      >
        <X class="size-4" />
      </button>
    </div>
    <!-- Wherever touch is available, every row and the close button reach 44px:
         `any-pointer`, not `pointer`, so a touchscreen laptop whose primary pointer is its
         trackpad still counts. A mouse-only narrow window keeps the denser list, since
         twenty rows at 44px scroll on most screens. -->
    <nav aria-label={s.shell.accountSections} class="flex flex-col gap-1">
      <AccountNavGroups variant="drawer" onnavigate={close} />
    </nav>
  </div>
{/if}
