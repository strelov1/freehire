<script lang="ts">
  import { MessageSquareText, X } from '@lucide/svelte';
  import { Button } from '$lib/ui';
  import { errorMessage } from '$lib/utils';
  import { focusTrap } from '$lib/actions/focusTrap';
  import { dynamicLabel, FACETS } from '$lib/facets';
  import { experienceLabel, freshnessLabel } from '$lib/filterControls';
  import { filtersFromInterpretation } from '$lib/aiFilter';
  import { api } from '$lib/api';
  import type { Interpretation } from '$lib/types';
  import type { FilterStore } from '$lib/filters';
  import { filtersToParams } from '$lib/facetModel';

  // Describe a search in words; the server turns it into filter values it has already
  // canonicalised against the real dictionaries. Nothing is applied until the preview
  // is accepted, and the preview names whatever the server could not place — a drop the
  // user is not told about is indistinguishable from a value that WAS applied.
  let { store, onclose }: { store: FilterStore; onclose: () => void } = $props();

  /** One previewed value. `exclude` draws it struck through, as the sidebar does. */
  type PreviewChip = { text: string; exclude: boolean };
  type PreviewGroup = { label: string; chips: PreviewChip[] };

  let text = $state('');
  let refinement = $state('');
  let result = $state.raw<Interpretation | null>(null); // an API payload, only reassigned
  let error = $state<string | null>(null);
  let busy = $state(false);

  const EXAMPLES = [
    'Senior Go backend, remote, somewhere in Europe, posted this week',
    'Fullstack with Node and React, remote, not in the USA',
    'ML engineer with NLP experience, startups only',
  ];

  // The chips the preview shows. Built from what RESOLVED, grouped the way the sidebar
  // groups them, so the preview and the applied filter read the same.
  const preview = $derived.by(() => {
    if (!result) return [];
    const label = (param: string, value: string) =>
      FACETS.find((d) => d.param === param)?.options?.find((o) => o.value === value)?.label ??
      dynamicLabel(param, value);
    const groups: PreviewGroup[] = [];
    const push = (heading: string, chips: PreviewChip[]) => {
      if (chips.length) groups.push({ label: heading, chips });
    };
    const one = (heading: string, text: string) => push(heading, [{ text, exclude: false }]);

    for (const def of FACETS) {
      // Included wins over excluded, the rule filtersFromInterpretation applies when the
      // same values become a filter. The server drops the overlap before it gets here,
      // but the chips are keyed by text and a duplicate key does not merely look odd in
      // Svelte — it breaks the whole each block. Cheap insurance on a rendered value.
      const included = result.facets?.[def.param] ?? [];
      push(def.label, [
        ...included.map((v) => ({ text: label(def.param, v), exclude: false })),
        ...(result.exclude?.[def.param] ?? [])
          .filter((v) => !included.includes(v))
          .map((v) => ({ text: label(def.param, v), exclude: true })),
      ]);
    }
    // The bounds use the SHARED labels, not wording of our own: the preview promises to
    // read the same as the chips it becomes, and "Last 7 days" beside a chip saying
    // "1 week" breaks that for nothing.
    if (result.query) one('Search', result.query);
    if (result.salary_min != null) one('Salary', `${result.salary_min.toLocaleString('en-US')}+`);
    if (result.posted_within_days != null) one('Posted', freshnessLabel(result.posted_within_days));
    if (result.experience_years_max != null) one('Experience', experienceLabel(result.experience_years_max));
    if (result.visa_sponsorship) one('Visa', 'Sponsorship');
    return groups;
  });

  async function build(description: string, previous?: Interpretation) {
    if (!description.trim() || busy) return;
    busy = true;
    error = null;
    try {
      result = await api.interpretSearch(description, previous);
      refinement = '';
    } catch (e) {
      error = errorMessage(e, "Couldn't build a filter from that.");
    } finally {
      busy = false;
    }
  }

  // Applying goes through the same query-string path a saved search uses, so an
  // AI-built filter and a hand-built one land identically: one URL write, ordinary
  // removable chips, and the existing Save filter control right beside them.
  function apply() {
    if (!result) return;
    store.apply(filtersToParams(filtersFromInterpretation(result)).toString());
    onclose();
  }
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && onclose()} />

<div class="fixed inset-0 z-50 flex items-center justify-center p-4">
  <button type="button" aria-label="Close dialog" class="absolute inset-0 bg-black/50" onclick={onclose}></button>

  <div
    role="dialog"
    aria-modal="true"
    aria-label="Describe your search"
    class="relative z-10 flex max-h-full w-full max-w-lg flex-col overflow-hidden rounded-2xl border border-border bg-background shadow-xl"
    {@attach focusTrap()}
  >
    <div class="flex items-start gap-3 border-b border-border p-5">
      <div class="flex size-10 shrink-0 items-center justify-center rounded-xl bg-primary/10">
        <MessageSquareText class="size-5 text-primary" />
      </div>
      <div class="min-w-0 flex-1">
        <h2 class="text-lg font-semibold leading-tight">Describe your search</h2>
        <p class="mt-0.5 text-sm text-muted-foreground">Say what you want in plain words — we set the filters.</p>
      </div>
      <button type="button" aria-label="Close" class="text-muted-foreground hover:text-foreground" onclick={onclose}>
        <X class="size-5" />
      </button>
    </div>

    <div class="flex-1 overflow-y-auto p-5">
      {#if !result}
        <textarea
          bind:value={text}
          rows="3"
          placeholder="Senior Go backend, remote, somewhere in Europe…"
          class="w-full rounded-xl border border-border bg-background p-3 text-sm outline-none focus:border-primary"
          onkeydown={(e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
              e.preventDefault();
              void build(text);
            }
          }}
        ></textarea>
        <p class="mt-3 text-xs font-medium text-muted-foreground">For example:</p>
        <ul class="mt-1 flex flex-col gap-1">
          {#each EXAMPLES as example (example)}
            <li>
              <button
                type="button"
                class="text-left text-xs text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
                onclick={() => (text = example)}
              >
                {example}
              </button>
            </li>
          {/each}
        </ul>
      {:else if result.empty}
        <!-- Nothing resolved. This is its own answer, not an empty filter: applying one
             would show the whole catalogue and read as "everything matches you". -->
        <p class="text-sm">
          I couldn't turn that into filters. Try naming a role, a skill or a place — for example
          "senior backend, Go, remote in Europe".
        </p>
        <Button class="mt-4" variant="outline" onclick={() => (result = null)}>Start over</Button>
      {:else}
        <!-- Dimmed while a refinement is in flight. The call takes tens of seconds and
             replaces everything below, so leaving the old answer looking live reads as
             "nothing happened" — which is exactly how the first version was reported. -->
        <div class={busy ? 'pointer-events-none opacity-40 transition-opacity' : ''}>
          <p class="text-sm font-medium">{result.summary}</p>

          <div class="mt-4 flex flex-col gap-3">
            {#each preview as group (group.label)}
              <div>
                <p class="text-xs font-medium text-muted-foreground">{group.label}</p>
                <div class="mt-1 flex flex-wrap gap-1.5">
                  {#each group.chips as chip (chip.text)}
                    <span
                      class="rounded-lg px-2 py-1 text-xs {chip.exclude
                        ? 'bg-destructive/10 text-destructive line-through'
                        : 'bg-muted'}"
                    >
                      {chip.text}
                    </span>
                  {/each}
                </div>
              </div>
            {/each}
          </div>

          {#if result.unresolved?.length}
            <p class="mt-4 text-xs text-muted-foreground">
              Didn't recognise: {result.unresolved.join(', ')}
            </p>
          {/if}
        </div>

        <!-- The refine action sits ON the field, not only in the footer. Typing alone
             does nothing, and a button at the far end of the dialog is not where anyone
             looks for the effect of what they just typed. -->
        <div class="mt-4 flex gap-2">
          <input
            bind:value={refinement}
            disabled={busy}
            placeholder="Add a constraint — e.g. remote only"
            class="min-w-0 flex-1 rounded-xl border border-border bg-background px-3 py-2 text-sm outline-none focus:border-primary disabled:opacity-50"
            onkeydown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault();
                void build(refinement, result ?? undefined);
              }
            }}
          />
          <Button variant="outline" disabled={busy || !refinement.trim()} onclick={() => build(refinement, result ?? undefined)}>
            {busy ? 'Rebuilding…' : 'Update'}
          </Button>
        </div>
        <p class="mt-1.5 text-xs text-muted-foreground">
          {#if busy}
            Rebuilding the whole search with that added — this takes a moment.
          {:else}
            Press Enter or Update. The search is rebuilt from scratch with your addition.
          {/if}
        </p>
      {/if}

      {#if error}
        <p class="mt-3 text-sm text-destructive">{error}</p>
      {/if}
    </div>

    <div class="flex justify-end gap-2 border-t border-border p-4">
      {#if !result}
        <Button disabled={busy || !text.trim()} onclick={() => build(text)}>
          {busy ? 'Building…' : 'Build filter'}
        </Button>
      {:else if !result.empty}
        <!-- Refining lives beside its own input above; the footer is only for the one
             action that ends the dialog. -->
        <Button disabled={busy} onclick={apply}>Apply</Button>
      {/if}
    </div>
  </div>
</div>
