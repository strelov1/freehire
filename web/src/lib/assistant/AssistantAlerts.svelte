<script lang="ts">
  import { AlertTriangle, X } from '@lucide/svelte';
  import type { TurnRefused } from '#lib/assistant/client.js';
  import PlanLimitLink from '#lib/components/PlanLimitLink.svelte';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, plural, t } from '#lib/i18n/t.js';
  import { messages } from './AssistantChat.messages';

  // AssistantChat's banners over the transcript: a failed request (`error`), a session that
  // reached its turn ceiling (`refusal`, with the control that extends it) and the CV
  // bullet-cap notice the candidate dismisses.
  let {
    error,
    refusal,
    extending,
    onContinue,
    bulletCapAlert,
    onDismissBulletCap,
  }: {
    error: { message: string; planLimitReached: boolean } | null;
    refusal: TurnRefused | null;
    extending: boolean;
    onContinue: () => void;
    bulletCapAlert: string | null;
    onDismissBulletCap: () => void;
  } = $props();

  const s = $derived(t(messages, locale()));
</script>

{#if error}
  <div
    class="m-3 mb-0 flex items-start gap-2 rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive-strong"
    role="alert"
  >
    <AlertTriangle class="mt-0.5 size-4 shrink-0" />
    <div class="flex min-w-0 flex-1 flex-col gap-1">
      <span>{error.message}</span>
      {#if error.planLimitReached}
        <!-- outline, not the default primary: this alert is already destructive-styled, and a
           filled brand-colored button would fight that treatment (see PlanLimitLink). -->
        <PlanLimitLink variant="outline" />
      {/if}
    </div>
  </div>
{/if}
{#if refusal}
  <div
    class="m-3 mb-0 flex items-start gap-3 rounded-lg border border-warning/40 bg-warning/5 px-3 py-2 text-sm"
    role="status"
  >
    <AlertTriangle class="mt-0.5 size-4 shrink-0 text-warning-strong" />
    <div class="flex min-w-0 flex-1 flex-col gap-1">
      <span>{refusal.message}</span>
      {#if refusal.ceiling > 0}
        <span class="text-xs text-muted-foreground">
          {format(plural(locale(), refusal.ceiling, s.turnsUsed), {
            turns: String(refusal.turns),
            ceiling: String(refusal.ceiling),
          })}
        </span>
      {/if}
    </div>
    <button
      type="button"
      class="shrink-0 rounded-md border border-border px-2 py-1 text-xs font-medium transition-colors hover:bg-muted disabled:opacity-60"
      onclick={onContinue}
      disabled={extending}
    >
      {extending ? s.continuingEllipsis : s.continueUsesSession}
    </button>
  </div>
{/if}
{#if bulletCapAlert}
  <div
    class="m-3 mb-0 flex items-start gap-2 rounded-lg border border-warning/40 bg-warning/5 px-3 py-2 text-sm text-foreground"
    role="status"
  >
    <AlertTriangle class="mt-0.5 size-4 shrink-0 text-warning-strong" />
    <span class="min-w-0 flex-1">{bulletCapAlert}</span>
    <button
      type="button"
      class="relative shrink-0 rounded-md p-0.5 text-muted-foreground transition-colors after:absolute after:-inset-0.5 hover:bg-muted hover:text-foreground"
      aria-label={s.dismiss}
      onclick={onDismissBulletCap}
    >
      <X class="size-4" />
    </button>
  </div>
{/if}
