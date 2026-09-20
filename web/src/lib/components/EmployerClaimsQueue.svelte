<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import { AsyncData } from '$lib/asyncData.svelte';
  import type { EmployerAccount } from '$lib/types';
  import { Button } from '$lib/ui';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { timeAgo } from '$lib/utils';
  import States from './States.svelte';

  // Company claims that could not auto-activate (an unknown or mismatched work-email
  // domain — see internal/ingest/employer/AGENTS.md). Keyed by user_id, the account's own
  // primary key and the id the approve/reject endpoints address.
  const pending = new AsyncData<EmployerAccount[]>([]);
  let busy = $state<number | null>(null);

  onMount(() => void pending.run(() => api.listPendingEmployerClaims()));

  async function approve(acc: EmployerAccount) {
    busy = acc.user_id;
    try {
      await api.approveEmployerClaim(acc.user_id);
      pending.value = pending.value.filter((a) => a.user_id !== acc.user_id);
    } catch {
      await pending.run(() => api.listPendingEmployerClaims());
    } finally {
      busy = null;
    }
  }

  async function reject(acc: EmployerAccount) {
    if (!window.confirm(`Reject the claim on "${acc.company_name}"? This frees the company slug for a future claim.`)) return;
    busy = acc.user_id;
    try {
      await api.rejectEmployerClaim(acc.user_id);
      pending.value = pending.value.filter((a) => a.user_id !== acc.user_id);
    } catch {
      await pending.run(() => api.listPendingEmployerClaims());
    } finally {
      busy = null;
    }
  }
</script>

{#if pending.status === 'loading'}
  <States state="loading" />
{:else if pending.status === 'error'}
  <States state="error" message="Couldn't load the queue." />
{:else if pending.value.length === 0}
  <States state="empty" message="No employer claims awaiting review." />
{:else}
  <ul class="flex flex-col divide-y divide-border rounded-lg border border-border">
    {#each pending.value as acc (acc.user_id)}
      <li class="flex flex-col gap-3 px-4 py-3 sm:flex-row sm:items-start sm:justify-between">
        <div class="flex min-w-0 flex-col gap-0.5">
          <span class="truncate text-sm font-medium">{acc.company_name}</span>
          <span class="truncate text-xs text-muted-foreground">
            {acc.work_email} · claimed {timeAgo(acc.created_at, locale())}
          </span>
        </div>
        <div class="flex shrink-0 gap-2">
          <Button variant="primary" size="sm" disabled={busy === acc.user_id} onclick={() => approve(acc)}>
            Approve
          </Button>
          <Button variant="ghost" size="sm" disabled={busy === acc.user_id} onclick={() => reject(acc)}>
            Reject
          </Button>
        </div>
      </li>
    {/each}
  </ul>
  <p class="mt-4 text-xs text-muted-foreground">
    Approving checks the work email is plausibly this company's own and, if the company's
    website was unknown, records the confirmed domain.
  </p>
{/if}
