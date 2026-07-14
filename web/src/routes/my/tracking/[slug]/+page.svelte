<script lang="ts">
  import JobRow from '$lib/components/JobRow.svelte';
  import { statusLabel, statusClass } from '$lib/emailStatus';
  import { timeAgo } from '$lib/utils';
  import { avatarInitials, avatarColor } from '$lib/avatar';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();
  const app = $derived(data.application);
</script>

<svelte:head>
  <title>{app.job.company} — application — freehire</title>
</svelte:head>

<div class="flex flex-col gap-5">
  <JobRow job={app.job} dimViewed={false} />

  <div class="flex flex-wrap items-center gap-2 text-sm">
    {#if app.stage}
      <span class="rounded-full border border-border px-2.5 py-0.5 text-xs capitalize">{app.stage}</span>
    {/if}
    {#if app.applied_at}
      <span class="text-xs text-muted-foreground">Applied {timeAgo(app.applied_at)}</span>
    {/if}
  </div>

  <div>
    <h2 class="mb-2 text-sm font-semibold">Linked emails ({app.emails.length})</h2>
    {#if app.emails.length === 0}
      <p class="text-sm text-muted-foreground">No emails linked to this application yet.</p>
    {:else}
      <ul class="flex flex-col gap-1">
        {#each app.emails as e (e.id)}
          <li class="flex items-start gap-3 rounded-xl border border-border p-3">
            <div
              class="mt-0.5 flex h-9 w-9 shrink-0 select-none items-center justify-center rounded-full text-xs font-semibold text-white"
              style="background-color: {avatarColor(e.from_addr || e.from_name)}"
            >
              {avatarInitials(e.from_name, e.from_addr)}
            </div>
            <div class="min-w-0 flex-1">
              <div class="flex items-baseline gap-2">
                <span class="min-w-0 flex-1 truncate text-sm font-medium">{e.from_name || e.from_addr}</span>
                <span class="shrink-0 text-[11px] text-muted-foreground">{timeAgo(e.received_at)}</span>
              </div>
              <div class="mt-0.5 truncate text-sm text-muted-foreground">{e.subject || '(no subject)'}</div>
              {#if statusLabel(e.status_signal)}
                <span class="mt-1 inline-block rounded border px-1.5 text-[10px] leading-4 {statusClass(e.status_signal)}">
                  {statusLabel(e.status_signal)}
                </span>
              {/if}
            </div>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</div>
