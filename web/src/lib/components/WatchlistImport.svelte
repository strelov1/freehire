<script lang="ts">
  import { ApiError, api } from '#lib/api.js';
  import type { WatchlistImportResult } from '#lib/types.js';
  import { Button } from '#lib/ui/index.js';

  const EXAMPLE = 'Stripe\nGitLab\nshopify';

  // Bulk add: company names or slugs, one per line. What did not match comes back into
  // the box, so a typo is fixed in place and the same button sends only what is left.
  let { max, onImported }: { max: number; onImported: () => void } = $props();

  let text = $state('');
  let busy = $state(false);
  let error = $state<string | null>(null);
  let result = $state<WatchlistImportResult | null>(null);

  const entries = $derived(
    text
      .split(/\r?\n/)
      .map((line) => line.trim())
      .filter((line) => line !== ''),
  );

  async function submit() {
    if (busy || entries.length === 0) return;
    if (entries.length > max) {
      error = `Paste at most ${max} companies at a time.`;
      return;
    }
    busy = true;
    error = null;
    result = null;
    try {
      result = await api.importWatchlist(entries);
      text = result.unmatched.join('\n');
      onImported();
    } catch (e) {
      error = e instanceof ApiError ? e.message : 'Could not import. Please try again.';
    } finally {
      busy = false;
    }
  }
</script>

<div class="flex flex-col gap-3">
  <label for="watchlist-import" class="text-sm font-medium">Company names, one per line</label>
  <textarea
    id="watchlist-import"
    bind:value={text}
    rows="6"
    placeholder={EXAMPLE}
    class="w-full rounded-lg border border-input bg-transparent px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring dark:bg-input/30"
  ></textarea>
  <div class="flex items-center gap-3">
    <Button variant="primary" size="sm" onclick={submit} disabled={busy || entries.length === 0}>
      {busy ? 'Importing…' : 'Import'}
    </Button>
    <span class="text-xs text-muted-foreground">Names or company page slugs, up to {max}.</span>
  </div>
  {#if error}
    <p class="text-sm text-destructive">{error}</p>
  {/if}
  {#if result}
    <div role="status" class="rounded-lg border border-border bg-muted/40 p-3 text-sm">
      <p>
        Added {result.added}, already on the list {result.already}, not matched {result.unmatched.length}.
      </p>
      {#if result.unmatched.length > 0}
        <p class="mt-2 text-muted-foreground">
          These did not match a company. They are back in the box above so you can fix them and import again:
        </p>
        <ul class="mt-1 list-disc pl-5">
          {#each result.unmatched as name (name)}
            <li>{name}</li>
          {/each}
        </ul>
      {/if}
    </div>
  {/if}
</div>
