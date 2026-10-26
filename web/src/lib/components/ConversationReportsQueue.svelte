<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '#lib/api.js';
  import type { ConversationReport } from '#lib/types.js';
  import { Button } from '#lib/ui/index.js';
  import { reportReasonLabel } from '#lib/messaging.js';
  import States from './States.svelte';
  // Reported conversations of every kind. Each participant is named the way the
  // conversation's kind draws them (a buddy by handle), never by account.
  let reports = $state<ConversationReport[]>([]);
  let loading = $state(true);
  let error = $state('');
  let acting = $state<number | null>(null);
  async function load() { loading = true; error = ''; try { reports = await api.listConversationReports(); } catch { error = 'Could not load conversation reports.'; } finally { loading = false; } }
  async function resolve(id: number) { acting = id; try { await api.resolveConversationReport(id); reports = reports.filter((r) => r.id !== id); } catch { error = 'Could not resolve report.'; } finally { acting = null; } }
  onMount(() => { void load(); });
</script>
{#if loading}<States state="loading" />{:else if error && reports.length === 0}<States state="error" message={error} onRetry={() => void load()} />{:else if reports.length === 0}<States state="empty" message="No open conversation reports." />{:else}
  <ul class="flex flex-col gap-4">{#each reports as report (report.id)}<li class="rounded-lg border border-border p-4">
    <div class="flex flex-wrap items-center justify-between gap-3"><strong>{reportReasonLabel(report.reason)}: {report.reporter} reported {report.reported}</strong><Button size="sm" variant="primary" disabled={acting !== null} onclick={() => resolve(report.id)}>Resolve</Button></div>
    {#if report.note}<p class="mt-2 text-sm">{report.note}</p>{/if}
    <ol class="mt-3 flex flex-col gap-2">{#each report.messages as message, index (index)}<li class="rounded bg-muted p-2 text-sm"><strong>{message.sender}:</strong> {message.body}<time class="ml-2 text-xs text-muted-foreground">{message.created_at}</time></li>{/each}</ol>
  </li>{/each}</ul>
{/if}
{#if error && reports.length > 0}<p role="alert" class="mt-3 text-sm text-destructive">{error}</p>{/if}
