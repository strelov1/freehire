<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { X } from '@lucide/svelte';
  import { isAuthenticated, currentUser } from '$lib/auth.svelte';
  import { visibleAccountNav, isSectionActive } from '$lib/accountNav';
  import { accountNavIcons } from '$lib/accountNavIcons';
  import { cn } from '$lib/utils';

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

  const path = $derived(page.url.pathname);
  const navItems = $derived(
    visibleAccountNav(currentUser()?.role === 'moderator', currentUser()?.beta_tester ?? false),
  );

  const close = () => (open = false);
  const onKey = (e: KeyboardEvent) => {
    if (open && e.key === 'Escape') close();
  };
</script>

{#snippet navLink(item: ReturnType<typeof visibleAccountNav>[number], withLabel: boolean)}
  {@const active = isSectionActive(path, item.href)}
  {@const Icon = accountNavIcons[item.href]}
  <a
    href={resolve(item.href)}
    aria-current={active ? 'page' : undefined}
    title={withLabel ? undefined : item.label}
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
      <span class="text-sm">{item.label}</span>
    {:else}
      <span class="sr-only">{item.label}</span>
    {/if}
  </a>
{/snippet}

<svelte:window onkeydown={onKey} />

{#if isAuthenticated()}
  {#if collapsible}
    <!-- Desktop icon rail (lg+): identical to the non-collapsible rail. -->
    <nav
      aria-label="Account sections"
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
        aria-label="Close menu"
        onclick={close}
        class="fixed inset-0 z-40 bg-black/40 lg:hidden"
      ></button>
      <nav
        aria-label="Account sections"
        class="fixed left-0 top-0 z-50 flex h-full w-64 flex-col gap-1 overflow-y-auto border-r border-border bg-background p-2 lg:hidden"
      >
        <div class="mb-1 flex items-center justify-between px-2 py-1.5">
          <span class="text-sm font-semibold text-foreground">Menu</span>
          <button
            type="button"
            aria-label="Close menu"
            onclick={close}
            class="rounded-md p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
          >
            <X class="size-4" />
          </button>
        </div>
        {#each navItems as item (item.href)}
          {@render navLink(item, true)}
        {/each}
      </nav>
    {/if}
  {:else}
    <nav
      aria-label="Account sections"
      class="flex w-14 shrink-0 flex-col gap-1 overflow-y-auto border-r border-border bg-background p-2"
    >
      {#each navItems as item (item.href)}
        {@render navLink(item, false)}
      {/each}
    </nav>
  {/if}
{/if}
