<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '#lib/api.js';
  import { AsyncData } from '#lib/asyncData.svelte.js';
  import type { EmployerAccount } from '#lib/types.js';
  import { Button, ConfirmDialog } from '#lib/ui/index.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { timeAgo } from '#lib/utils.js';
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

  // The claim awaiting the reject confirmation; the dialog is open while it is set.
  let rejectTarget = $state<EmployerAccount | null>(null);
  let confirmRejectOpen = $state(false);

  function askReject(acc: EmployerAccount) {
    rejectTarget = acc;
    confirmRejectOpen = true;
  }

  async function reject(acc: EmployerAccount) {
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
          <Button variant="ghost" size="sm" disabled={busy === acc.user_id} onclick={() => askReject(acc)}>
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

<ConfirmDialog
  bind:open={confirmRejectOpen}
  title={`Reject the claim on "${rejectTarget?.company_name ?? ''}"?`}
  description="This frees the company slug for a future claim."
  confirmLabel="Reject"
  variant="destructive"
  onConfirm={() => (rejectTarget ? reject(rejectTarget) : undefined)}
/>
