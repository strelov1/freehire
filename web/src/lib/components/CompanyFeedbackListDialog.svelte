<script lang="ts">
  import { onMount } from 'svelte';
  import { Star } from '@lucide/svelte';
  import { api } from '$lib/api';
  import { companyFeedbackTypeLabel } from '$lib/companyFeedback';
  import type { CompanyFeedback } from '$lib/types';
  import { Button, Dialog } from '$lib/ui';
  import { formatDate } from '$lib/utils';

  // Read-only list of a company's feedback, offset-paginated — structurally like
  // RequestReferralModal's onMount-fetch-into-dialog shape, but with no form. The
  // "Write feedback" button is the list's own entry point into the write dialog:
  // without it, once a company has any feedback, the header badge only ever opens
  // this read-only list and there is no way back to CompanyFeedbackDialog.
  let {
    slug,
    companyName,
    onClose,
    onWriteFeedback,
  }: { slug: string; companyName?: string; onClose: () => void; onWriteFeedback: () => void } =
    $props();

  let open = $state(true);
  $effect(() => {
    if (!open) onClose();
  });

  const pageSize = 10;
  let items = $state<CompanyFeedback[]>([]);
  let offset = $state(0);
  let total = $state<number | undefined>(undefined);
  let hasMore = $state(false);
  let loading = $state(true);
  let loadingMore = $state(false);
  let error = $state<string | null>(null);

  async function loadPage() {
    const slice = await api.listCompanyFeedback(slug, pageSize, offset);
    items = [...items, ...slice.items];
    total = slice.total;
    hasMore = slice.hasMore && slice.items.length > 0;
    offset += slice.items.length;
  }

  onMount(async () => {
    try {
      await loadPage();
    } catch {
      error = 'Could not load feedback. Please try again.';
    } finally {
      loading = false;
    }
  });

  async function loadMore() {
    loadingMore = true;
    error = null;
    try {
      await loadPage();
    } catch {
      error = 'Could not load more feedback. Please try again.';
    } finally {
      loadingMore = false;
    }
  }
</script>

<Dialog bind:open title={companyName ? `Feedback on ${companyName}` : 'Feedback'} class="max-w-lg">
  {#if !loading}
    <div class="mb-4 flex justify-end">
      <Button variant="outline" size="sm" onclick={onWriteFeedback}>Write feedback</Button>
    </div>
  {/if}
  {#if loading}
    <p class="text-sm text-muted-foreground">Loading…</p>
  {:else if error && items.length === 0}
    <p role="alert" class="text-sm text-destructive">{error}</p>
  {:else if items.length === 0}
    <p class="text-sm text-muted-foreground">No feedback yet.</p>
  {:else}
    <ul class="flex max-h-[60vh] flex-col gap-4 overflow-y-auto">
      {#each items as item (item.id)}
        <li class="border-b border-border pb-4 last:border-b-0 last:pb-0">
          <div class="flex items-center gap-2">
            <div class="flex items-center text-amber-500">
              {#each [1, 2, 3, 4, 5] as value (value)}
                <Star class="size-4" fill={value <= item.rating ? 'currentColor' : 'none'} />
              {/each}
            </div>
            <span class="rounded-full bg-secondary px-2 py-0.5 text-xs text-secondary-foreground">
              {companyFeedbackTypeLabel(item.feedback_type)}
            </span>
          </div>
          <p class="mt-2 whitespace-pre-wrap text-sm">{item.body}</p>
          <p class="mt-1.5 text-xs text-muted-foreground">{item.author} · {formatDate(item.created_at)}</p>
        </li>
      {/each}
    </ul>
    {#if error}
      <p role="alert" class="mt-4 text-sm text-destructive">{error}</p>
    {/if}
    {#if hasMore}
      <Button variant="outline" class="mt-4 w-full" disabled={loadingMore} onclick={loadMore}>
        {loadingMore ? 'Loading…' : `Load more (${(total ?? 0) - items.length} more)`}
      </Button>
    {/if}
  {/if}
</Dialog>
