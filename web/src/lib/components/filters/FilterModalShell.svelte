<script lang="ts">
  import type { Snippet } from 'svelte';
  import { X } from '@lucide/svelte';
  import type { RailEntry, RailSection } from '$lib/filterSections';

  // The reusable two-pane filter-modal chrome: backdrop, header, the sectioned left
  // rail, and the deferred footer (Clear all / Apply / live preview). It knows nothing
  // about jobs vs companies — a domain wrapper (FilterModal, CompanyFilterModal) passes
  // the rail, a `staged` edit store, per-entry counts, a `seed` thunk run on open, an
  // `apply` action, and a `pane` snippet that renders the active entry's controls.
  //
  // Deferred-apply contract: the pane edits the caller's staged copy; nothing reaches
  // the live list until `apply` runs on the footer button.

  // The minimal staged-store surface the shell drives; the wrapper's StagedFilters /
  // StagedCompanyFilters both satisfy it (seed/commit are closed over by the wrapper's
  // `seed`/`apply` thunks, so the shell stays domain-agnostic).
  interface StagedSurface {
    readonly active: number;
    params(): URLSearchParams;
    clear(): void;
  }

  let {
    open = false,
    onClose,
    title = 'All filters',
    rail,
    sections,
    staged,
    entryCount,
    seed,
    apply,
    applyDisabled = false,
    applyLabel,
    previewCount,
    initialKey,
    pane,
    extra,
  }: {
    open?: boolean;
    onClose: () => void;
    title?: string;
    rail: RailEntry[];
    sections: RailSection[];
    staged: StagedSurface;
    entryCount: (e: RailEntry) => number;
    /** Seed the staged copy from the live state — run once per open. */
    seed: () => void;
    /** Commit the staged copy (or a custom save). A thrown error keeps the modal open. */
    apply: () => void | Promise<void>;
    applyDisabled?: boolean;
    /** Custom footer label (e.g. "Save"); when absent the button shows the live preview. */
    applyLabel?: string;
    previewCount?: (params: URLSearchParams) => Promise<number>;
    /** Rail entry to land on when the modal opens (defaults to the first entry) — lets a
     *  caller keep a lead tab (e.g. "My filters") in the rail without opening on it. */
    initialKey?: string;
    /** Renders the active entry's controls into the right pane. */
    pane: Snippet<[RailEntry]>;
    /** Optional content above the pane (e.g. the profile editor's "import from CV"). */
    extra?: Snippet;
  } = $props();

  // Set on the first open (see the seed effect); until then activeEntry falls back to
  // the first rail entry.
  let active = $state<string>('');

  // Seed the staged copy from the live state each time the modal opens, and land on the
  // `initialKey` entry (falling back to the first) when the current one isn't in this rail.
  let wasOpen = false;
  $effect(() => {
    if (open && !wasOpen) {
      seed();
      if (!rail.some((e) => e.key === active)) {
        active = (initialKey && rail.some((e) => e.key === initialKey) ? initialKey : rail[0]?.key) ?? '';
      }
    }
    wasOpen = open;
  });

  const activeEntry = $derived(rail.find((e) => e.key === active) ?? rail[0]);

  // Debounced live preview for the staged filters, keyed by a monotonic gen so a slow
  // response can't overwrite a newer one.
  let showCount = $state<number | null>(null);
  let countGen = 0;
  $effect(() => {
    if (!open || !previewCount) return;
    const params = staged.params(); // tracks the staged edits
    const gen = ++countGen;
    const pc = previewCount;
    const t = setTimeout(() => {
      pc(params)
        .then((n) => {
          if (gen === countGen) showCount = n;
        })
        .catch(() => {});
    }, 200);
    return () => clearTimeout(t);
  });

  let applyBusy = $state(false);
  let applyError = $state<string | null>(null);

  async function runApply() {
    if (applyDisabled || applyBusy) return;
    applyBusy = true;
    applyError = null;
    try {
      await apply();
      onClose();
    } catch (e) {
      applyError = e instanceof Error ? e.message : 'Something went wrong. Please try again.';
    } finally {
      applyBusy = false;
    }
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') onClose();
  }
</script>

<svelte:window onkeydown={open ? onKeydown : undefined} />

{#if open}
  <div class="fixed inset-0 z-50 flex items-stretch justify-center sm:items-center sm:p-6">
    <!-- backdrop -->
    <button class="absolute inset-0 bg-foreground/35" aria-label="Close filters" onclick={onClose}></button>

    <div
      class="relative flex h-full w-full flex-col overflow-hidden bg-background shadow-2xl sm:h-auto sm:max-h-[calc(100vh-3rem)] sm:max-w-4xl sm:rounded-2xl sm:border sm:border-border"
      role="dialog"
      aria-modal="true"
      aria-label={title}
    >
      <!-- header -->
      <div class="flex items-center gap-3 border-b border-border px-4 py-3">
        <h2 class="flex-1 text-base font-semibold tracking-tight">{title}</h2>
        <button
          type="button"
          onclick={onClose}
          aria-label="Close"
          class="flex size-9 items-center justify-center rounded-lg border border-border text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
        >
          <X class="size-4" />
        </button>
      </div>

      <div class="flex min-h-0 flex-1">
        <!-- rail -->
        <nav class="w-40 shrink-0 overflow-y-auto border-r border-border py-2 sm:w-56">
          {#each sections as section (section)}
            {@const entries = rail.filter((e) => e.section === section)}
            {#if entries.length}
              <div class="px-3 pb-1 pt-3 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">{section}</div>
              {#each entries as e (e.key)}
                {@const n = entryCount(e)}
                <button
                  type="button"
                  onclick={() => (active = e.key)}
                  class={[
                    'flex w-full items-center justify-between gap-2 border-l-2 px-3 py-2 text-left text-sm transition-colors',
                    active === e.key
                      ? 'border-foreground bg-accent font-semibold'
                      : 'border-transparent font-medium hover:bg-accent',
                  ]}
                >
                  <span class="truncate">{e.label}</span>
                  {#if n > 0}
                    <span
                      class={[
                        'shrink-0 rounded-full px-1.5 text-[11px] font-semibold',
                        active === e.key ? 'bg-primary text-primary-foreground' : 'bg-secondary text-muted-foreground',
                      ]}>{n}</span
                    >
                  {/if}
                </button>
              {/each}
            {/if}
          {/each}
        </nav>

        <!-- pane -->
        <section class="min-w-0 flex-1 overflow-y-auto p-4 sm:p-6">
          {#if extra}<div class="mb-6">{@render extra()}</div>{/if}
          {#if activeEntry}{@render pane(activeEntry)}{/if}
        </section>
      </div>

      <!-- footer -->
      <div class="flex flex-col gap-2 border-t border-border px-4 py-3">
        {#if applyError}
          <p class="text-sm text-destructive">{applyError}</p>
        {/if}
        <div class="flex items-center justify-between">
          <button
            type="button"
            onclick={() => staged.clear()}
            class="text-sm font-medium text-muted-foreground transition-colors hover:text-foreground">Clear all</button
          >
          <button
            type="button"
            onclick={runApply}
            disabled={applyDisabled || applyBusy}
            class="inline-flex h-11 items-center gap-1.5 rounded-lg bg-primary px-6 text-sm font-semibold text-primary-foreground transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {#if applyLabel}
              {applyBusy ? 'Saving…' : applyLabel}
            {:else}
              Show {showCount != null ? showCount.toLocaleString('en-US') : ''}
              {showCount === 1 ? 'job' : 'jobs'}
            {/if}
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}
