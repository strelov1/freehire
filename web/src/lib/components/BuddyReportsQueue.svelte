<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '#lib/api.js';
  import type { BuddyReport } from '#lib/types.js';
  import { Button } from '#lib/ui/index.js';
  import { buddyReportReasonLabel } from '#lib/buddies.js';
  import States from './States.svelte';
  let reports = $state<BuddyReport[]>([]);
  let loading = $state(true);
  let error = $state('');
  let acting = $state<number | null>(null);
  async function load() { loading = true; error = ''; try { reports = await api.listBuddyReports(); } catch { error = 'Could not load buddy reports.'; } finally { loading = false; } }
  async function resolve(id: number) { acting = id; try { await api.resolveBuddyReport(id); reports = reports.filter((r) => r.id !== id); } catch { error = 'Could not resolve report.'; } finally { acting = null; } }
  onMount(() => { void load(); });
</script>
{#if loading}<States state="loading" />{:else if error && reports.length === 0}<States state="error" message={error} onRetry={() => void load()} />{:else if reports.length === 0}<States state="empty" message="No open buddy reports." />{:else}
  <ul class="flex flex-col gap-4">{#each reports as report (report.id)}<li class="rounded-lg border border-border p-4">
    <div class="flex flex-wrap items-center justify-between gap-3"><strong>{buddyReportReasonLabel(report.reason)}: {report.reporter_handle} reported {report.reported_handle}</strong><Button size="sm" variant="primary" disabled={acting !== null} onclick={() => resolve(report.id)}>Resolve</Button></div>
    {#if report.note}<p class="mt-2 text-sm">{report.note}</p>{/if}
    <ol class="mt-3 flex flex-col gap-2">{#each report.messages as message, index (index)}<li class="rounded bg-muted p-2 text-sm"><strong>{message.sender_handle}:</strong> {message.body}<time class="ml-2 text-xs text-muted-foreground">{message.created_at}</time></li>{/each}</ol>
  </li>{/each}</ul>
{/if}
{#if error && reports.length > 0}<p role="alert" class="mt-3 text-sm text-destructive">{error}</p>{/if}
