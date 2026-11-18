<script lang="ts">
  import { onMount } from 'svelte';
  import { resolve } from '$app/paths';
  import { api, ApiError } from '#lib/api.js';
  import { announce } from '#lib/announce.svelte.js';
  import { AsyncData } from '#lib/asyncData.svelte.js';
  import type { PendingCompanyRelation } from '#lib/types.js';
  import { Button, ConfirmDialog } from '#lib/ui/index.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { timeAgo } from '#lib/utils.js';
  import States from './States.svelte';

  // Company relations people proposed (internal/job/companyrelation). Approving `same`
  // merges the first company into the second: its jobs move, and its page redirects. That
  // moves a public URL, so it asks for confirmation. Approving `subsidiary` only links the
  // two pages.
  const pending = new AsyncData<PendingCompanyRelation[]>([]);
  let busy = $state<number | null>(null);
  let error = $state<string | null>(null);

  onMount(() => void pending.run(() => api.listPendingCompanyRelations()));

  function drop(id: number) {
    pending.value = pending.value.filter((p) => p.id !== id);
  }

  async function act(p: PendingCompanyRelation, apply: boolean) {
    busy = p.id;
    error = null;
    try {
      if (apply) {
        const res = await api.approveCompanyRelation(p.id);
        announce(
          res.kind === 'same'
            ? `Merged ${p.company_name} into ${p.target_name}, ${res.moved_jobs} jobs moved`
            : `Linked ${p.company_name} under ${p.target_name}`,
        );
      } else {
        await api.rejectCompanyRelation(p.id);
      }
      drop(p.id);
    } catch (err) {
      error = err instanceof ApiError ? err.message : 'Something went wrong. Please try again.';
      announce(error, 'assertive');
    } finally {
      busy = null;
    }
  }

  // The merge awaiting confirmation; the dialog is open while it is set.
  let mergeTarget = $state<PendingCompanyRelation | null>(null);
  let confirmMergeOpen = $state(false);

  function approve(p: PendingCompanyRelation) {
    if (p.kind === 'same' && p.status === 'pending') {
      mergeTarget = p;
      confirmMergeOpen = true;
      return;
    }
    void act(p, true);
  }
</script>

{#snippet company(slug: string, name: string, jobs: number)}
  {#if name}
    <a class="font-medium text-primary hover:underline" href={resolve('/companies/[slug]', { slug })}>{name}</a>
    <span class="text-xs text-muted-foreground">({jobs} open)</span>
  {:else}
    <span class="font-medium">{slug}</span>
    <span class="text-xs text-muted-foreground">(no company row any more)</span>
  {/if}
{/snippet}

{#if pending.status === 'loading'}
  <States state="loading" />
{:else if pending.status === 'error'}
  <States state="error" message="Couldn't load the queue." />
{:else if pending.value.length === 0}
  <States state="empty" message="No company corrections awaiting review." />
{:else}
  {#if error}
    <p role="alert" class="mb-3 text-sm text-destructive">{error}</p>
  {/if}
  <ul class="flex flex-col divide-y divide-border rounded-lg border border-border">
    {#each pending.value as p (p.id)}
      <li class="flex flex-col gap-3 px-4 py-3 sm:flex-row sm:items-start sm:justify-between">
        <div class="flex min-w-0 flex-col gap-1 text-sm">
          <span>
            {@render company(p.company_slug, p.company_name, p.company_job_count)}
            {p.kind === 'same' ? 'is the same employer as' : 'is a subsidiary of'}
            {@render company(p.target_slug, p.target_name, p.target_job_count)}
          </span>
          {#if p.note}
            <span class="whitespace-pre-line text-muted-foreground">{p.note}</span>
          {/if}
          <span class="text-xs text-muted-foreground">proposed {timeAgo(p.created_at, locale())}</span>
          {#if p.status === 'approved'}
            <span class="text-xs text-destructive"
              >Approved, but its jobs did not all move. Resume to finish the merge.</span
            >
          {/if}
        </div>
        <div class="flex shrink-0 gap-2">
          <Button variant="primary" size="sm" disabled={busy !== null} onclick={() => approve(p)}>
            {p.status === 'approved' ? 'Resume' : p.kind === 'same' ? 'Merge' : 'Link'}
          </Button>
          {#if p.status === 'pending'}
            <Button variant="ghost" size="sm" disabled={busy !== null} onclick={() => act(p, false)}>Reject</Button>
          {/if}
        </div>
      </li>
    {/each}
  </ul>
  <p class="mt-4 text-xs text-muted-foreground">
    A merge moves the first company's jobs to the second and redirects its page there. It is recorded as a moderator
    decision, separate from the automatic merges. The search index catches up on the next scheduled reindex.
  </p>
{/if}

<ConfirmDialog
  bind:open={confirmMergeOpen}
  title={`Merge "${mergeTarget?.company_name ?? ''}" into "${mergeTarget?.target_name ?? ''}"?`}
  description="Its jobs move to the second company and its page redirects there. Make sure they are the same employer, not a parent and a subsidiary."
  confirmLabel="Merge"
  onConfirm={() => (mergeTarget ? act(mergeTarget, true) : undefined)}
/>
