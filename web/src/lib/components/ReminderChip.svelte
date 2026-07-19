<script lang="ts">
  import { Bell, BellOff } from '@lucide/svelte';
  import { api, ApiError } from '$lib/api';
  import { cn, timeAgo } from '$lib/utils';

  // The per-saved-job reminder control: shows the pending reminder's countdown with
  // an inline (non-modal) panel to reschedule it to a new delay or turn it off. When
  // no reminder is pending, it offers to set one — re-saving with an override
  // schedules it without unsaving the job.

  let { slug, fireAt: initialFireAt }: { slug: string; fireAt: string | null } = $props();

  let fireAt = $state(initialFireAt);
  let open = $state(false);
  let busy = $state(false);
  let error = $state<string | null>(null);

  const presets = [1, 3, 7, 14];
  const label = (d: number) => (d === 1 ? '1 day' : `${d} days`);

  async function reschedule(days: number) {
    if (busy) return;
    busy = true;
    error = null;
    try {
      // A pending reminder is rescheduled; a saved job without one gets a fresh
      // reminder by re-saving with an override (idempotent, stays saved).
      if (fireAt) await api.rescheduleReminder(slug, days);
      else await api.saveJob(slug, { delay_days: days });
      fireAt = new Date(Date.now() + days * 86_400_000).toISOString();
      open = false;
    } catch (e) {
      error = e instanceof ApiError ? e.message : 'Could not update the reminder.';
    } finally {
      busy = false;
    }
  }

  async function turnOff() {
    if (busy) return;
    busy = true;
    error = null;
    try {
      await api.cancelReminder(slug);
      fireAt = null;
      open = false;
    } catch (e) {
      error = e instanceof ApiError ? e.message : 'Could not turn off the reminder.';
    } finally {
      busy = false;
    }
  }

  const chipClass = (on: boolean) =>
    cn(
      'inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-medium transition-colors disabled:opacity-50',
      on
        ? 'border-transparent bg-brand-muted text-brand-strong'
        : 'border-border bg-background text-muted-foreground hover:border-muted-foreground/40',
    );
</script>

<div class="flex flex-col gap-1.5">
  <div class="flex flex-wrap items-center gap-2">
    <button type="button" onclick={() => (open = !open)} aria-expanded={open} disabled={busy} class={chipClass(fireAt != null)}>
      {#if fireAt}
        <Bell class="size-3.5" aria-hidden="true" /> Reminder {timeAgo(fireAt)}
      {:else}
        <Bell class="size-3.5" aria-hidden="true" /> Remind me
      {/if}
    </button>
  </div>

  {#if open}
    <div class="flex flex-wrap items-center gap-1.5">
      {#each presets as d (d)}
        <button type="button" onclick={() => reschedule(d)} disabled={busy} class={chipClass(false)}>{label(d)}</button>
      {/each}
      {#if fireAt}
        <button type="button" onclick={turnOff} disabled={busy} class="inline-flex items-center gap-1.5 rounded-full border border-border bg-background px-2.5 py-1 text-xs font-medium text-muted-foreground transition-colors hover:border-destructive/40 hover:text-destructive disabled:opacity-50">
          <BellOff class="size-3.5" aria-hidden="true" /> Off
        </button>
      {/if}
    </div>
  {/if}

  {#if error}
    <p class="text-xs text-destructive">{error}</p>
  {/if}
</div>
