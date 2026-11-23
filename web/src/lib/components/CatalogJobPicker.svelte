<script lang="ts">
  import { ArrowRight, Search } from '@lucide/svelte';
  import { api } from '#lib/api.js';
  import type { ToolJob } from '#lib/api-methods/jobs.js';
  import { Button, Input } from '#lib/ui/index.js';

  let { onSelect }: { onSelect: (job: ToolJob) => void } = $props();

  const inputId = $props.id();
  let query = $state('');
  let result = $state.raw<ToolJob[] | 'searching' | 'error'>([]);
  let searchAttempt = $state(0);

  $effect(() => {
    const q = query.trim();
    result = q ? 'searching' : [];
    if (!q) return;

    // Retry the same query when the visitor asks, and cancel superseded searches.
    void searchAttempt;
    const controller = new AbortController();
    const timer = setTimeout(() => runSearch(q, controller.signal), 250);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  });

  async function runSearch(q: string, signal: AbortSignal) {
    try {
      const { items } = await api.searchJobs(new URLSearchParams({ q, q_fields: 'title,company' }), 8, 0, signal);
      if (!signal.aborted) result = items;
    } catch {
      if (!signal.aborted) result = 'error';
    }
  }
</script>

<label for={inputId} class="text-sm font-medium">Find a job</label>
<div class="relative mt-2">
  <Search
    class="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground"
    aria-hidden="true"
  />
  <Input
    id={inputId}
    bind:value={query}
    type="search"
    autocomplete="off"
    placeholder="Search by job title or company"
    maxlength={200}
    class="w-full pl-9"
  />
</div>
<div aria-live="polite">
  {#if result === 'searching'}
    <p class="mt-2 text-sm text-muted-foreground">Searching jobs…</p>
  {:else if result === 'error'}
    <div class="mt-2 flex flex-wrap items-center gap-2">
      <p class="text-sm text-destructive">We couldn’t search jobs.</p>
      <Button size="sm" variant="outline" onclick={() => searchAttempt++}>Try again</Button>
    </div>
  {:else if result.length > 0}
    <ul class="mt-2 divide-y divide-border rounded-lg border border-border">
      {#each result as option (option.public_slug)}
        <li>
          <button
            type="button"
            class="flex w-full items-center justify-between gap-3 px-3 py-2 text-left hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
            onclick={() => onSelect(option)}
          >
            <span class="min-w-0 wrap-break-word">
              <span class="block text-sm font-medium">{option.title}</span>
              {#if option.company || option.location}
                <span class="block text-sm text-muted-foreground"
                  >{[option.company, option.location].filter(Boolean).join(' · ')}</span
                >
              {/if}
            </span>
            <ArrowRight class="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
          </button>
        </li>
      {/each}
    </ul>
  {:else if query.trim()}
    <p class="mt-2 text-sm text-muted-foreground">No jobs found. Try another title or company.</p>
  {/if}
</div>
