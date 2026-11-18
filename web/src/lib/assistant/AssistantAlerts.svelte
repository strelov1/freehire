<script lang="ts">
  import { AlertTriangle, X } from '@lucide/svelte';
  import { extendSession, TurnRefused } from '#lib/assistant/client.js';
  import type { ChatAlerts } from './chatAlerts.svelte';
  import PlanLimitLink from '#lib/components/PlanLimitLink.svelte';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, plural, t } from '#lib/i18n/t.js';
  import { messages } from './AssistantChat.messages';

  // AssistantChat's banners over the transcript: a failed request (`error`), a session that
  // reached its turn ceiling (`refusal`, with the control that extends it) and the CV
  // bullet-cap notice the candidate dismisses.
  let { alerts }: { alerts: ChatAlerts } = $props();

  const s = $derived(t(messages, locale()));

  let extending = $state(false);

  /** Spend another of today's tailoring sessions so this conversation can carry on.
   *
   *  It does not re-send the message: the draft was handed back when the turn was refused,
   *  so the candidate sends it themselves and can change their mind first. Extending is
   *  idempotent on the server, so a double click costs one session rather than two. */
  async function continueSession() {
    const stopped = alerts.refusal;
    if (!stopped || extending) return;
    extending = true;
    try {
      await extendSession(stopped.sessionId);
      alerts.refusal = null;
    } catch (err) {
      // Refused again means the day's tailoring allowance is gone too, and that IS a wall
      // until tomorrow — so it becomes an ordinary error rather than another offer, with the
      // same link to the plan the daily-limit refusal in AssistantChat's dispatch() shows.
      alerts.refusal = null;
      if (err instanceof TurnRefused) {
        alerts.limit(err.message);
      } else {
        alerts.fail(err instanceof Error ? err.message : s.continueFailed);
      }
    } finally {
      extending = false;
    }
  }
</script>

{#if alerts.error}
  <div
    class="m-3 mb-0 flex items-start gap-2 rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive-strong"
    role="alert"
  >
    <AlertTriangle class="mt-0.5 size-4 shrink-0" />
    <div class="flex min-w-0 flex-1 flex-col gap-1">
      <span>{alerts.error.message}</span>
      {#if alerts.error.planLimitReached}
        <!-- outline, not the default primary: this alert is already destructive-styled, and a
           filled brand-colored button would fight that treatment (see PlanLimitLink). -->
        <PlanLimitLink variant="outline" />
      {/if}
    </div>
  </div>
{/if}
{#if alerts.refusal}
  <div
    class="m-3 mb-0 flex items-start gap-3 rounded-lg border border-warning/40 bg-warning/5 px-3 py-2 text-sm"
    role="status"
  >
    <AlertTriangle class="mt-0.5 size-4 shrink-0 text-warning-strong" />
    <div class="flex min-w-0 flex-1 flex-col gap-1">
      <span>{alerts.refusal.message}</span>
      {#if alerts.refusal.ceiling > 0}
        <span class="text-xs text-muted-foreground">
          {format(plural(locale(), alerts.refusal.ceiling, s.turnsUsed), {
            turns: String(alerts.refusal.turns),
            ceiling: String(alerts.refusal.ceiling),
          })}
        </span>
      {/if}
    </div>
    <button
      type="button"
      class="shrink-0 rounded-md border border-border px-2 py-1 text-xs font-medium transition-colors hover:bg-muted disabled:opacity-60"
      onclick={continueSession}
      disabled={extending}
    >
      {extending ? s.continuingEllipsis : s.continueUsesSession}
    </button>
  </div>
{/if}
{#if alerts.bulletCap}
  <div
    class="m-3 mb-0 flex items-start gap-2 rounded-lg border border-warning/40 bg-warning/5 px-3 py-2 text-sm text-foreground"
    role="status"
  >
    <AlertTriangle class="mt-0.5 size-4 shrink-0 text-warning-strong" />
    <span class="min-w-0 flex-1">{alerts.bulletCap}</span>
    <button
      type="button"
      class="relative shrink-0 rounded-md p-0.5 text-muted-foreground transition-colors after:absolute after:-inset-0.5 hover:bg-muted hover:text-foreground"
      aria-label={s.dismiss}
      onclick={() => (alerts.bulletCap = null)}
    >
      <X class="size-4" />
    </button>
  </div>
{/if}
