<script lang="ts">
  // The two email groups the notification card above does not already own: the
  // saved-search digests, and the occasional letters about the product.
  //
  // Activity — reminders, follow-ups, report outcomes — is deliberately absent,
  // because ReminderSettings is already its switch and two controls for one boolean
  // is how a settings page starts lying. Together the two cards cover exactly what
  // the public /unsubscribe page offers, which is the point: somebody who signs in
  // must not find fewer controls than a link in an email gave them.
  import { onMount } from 'svelte';
  import { Mail } from '@lucide/svelte';
  import ToggleSwitch from '$lib/components/ToggleSwitch.svelte';
  import { api, type EmailPrefs } from '$lib/api';

  let prefs = $state<EmailPrefs | null>(null);
  let failedToLoad = $state(false);
  let saving = $state(false);
  let saveFailed = $state(false);

  onMount(async () => {
    try {
      prefs = await api.getMyEmailGroups();
    } catch {
      failedToLoad = true;
    }
  });

  async function save(update: Parameters<typeof api.saveMyEmailGroups>[0]) {
    saving = true;
    saveFailed = false;
    try {
      prefs = await api.saveMyEmailGroups(update);
    } catch {
      saveFailed = true;
    } finally {
      saving = false;
    }
  }
</script>

<section class="rounded-xl border border-border bg-card p-4">
  <div class="flex items-center gap-3">
    <div class="grid size-9 shrink-0 place-items-center rounded-lg bg-brand-muted text-brand-strong">
      <Mail class="size-4.5" aria-hidden="true" />
    </div>
    <div class="min-w-0 flex-1">
      <h2 class="text-sm font-semibold leading-tight">Email</h2>
      <p class="text-xs text-muted-foreground">
        The other two kinds of email we send. Security emails — confirming your address,
        resetting your password — are always sent.
      </p>
    </div>
    {#if saveFailed}
      <span class="text-xs text-destructive">Didn't save</span>
    {/if}
  </div>

  {#if failedToLoad}
    <p class="mt-4 text-xs text-destructive">Couldn't load your email settings.</p>
  {:else if prefs}
    <div class="mt-4 space-y-4">
      {@render row(
        'Job alerts',
        'New matches for the searches you saved.',
        prefs.alerts_enabled,
        () => save({ alerts: !prefs?.alerts_enabled }),
      )}
      {@render row(
        'News and product updates',
        'Occasional letters about what we’ve built.',
        prefs.news_enabled,
        () => save({ news: !prefs?.news_enabled }),
      )}
    </div>
  {/if}
</section>

{#snippet row(title: string, description: string, on: boolean, toggle: () => void)}
  <div class="flex items-start justify-between gap-4">
    <div>
      <p class="text-sm font-medium leading-tight">{title}</p>
      <p class="mt-0.5 text-xs text-muted-foreground">{description}</p>
    </div>
    <ToggleSwitch {on} label={title} disabled={saving} onToggle={toggle} />
  </div>
{/snippet}
