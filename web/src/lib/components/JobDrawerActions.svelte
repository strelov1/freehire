<script lang="ts">
  import { Send, SquarePen } from '@lucide/svelte';
  import { Button, cn } from '#lib/ui/index.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { messages } from './JobDrawer.messages';

  // The job drawer's stage and its two actions: tailor a CV, follow up a silent application.
  let {
    stageLabel,
    canTailor,
    tailorIsPrimary,
    tailoring,
    onTailor,
    offersFollowUp,
    followedUp,
    onFollowUp,
  }: {
    stageLabel: string | null;
    canTailor: boolean;
    tailorIsPrimary: boolean;
    tailoring: boolean;
    onTailor: () => void;
    offersFollowUp: boolean;
    followedUp: boolean;
    onFollowUp: () => void;
  } = $props();

  const s = $derived(t(messages, locale()));

  // Actions keep their natural width, including when only Tailor CV is offered.
  const actionClass = 'h-9 min-w-0 gap-1.5 px-3 sm:h-8';
</script>

{#if stageLabel || canTailor || offersFollowUp}
  <div class="flex flex-wrap items-center gap-x-3 gap-y-2">
    {#if stageLabel}
      <span class="inline-flex items-center gap-1.5 text-xs font-medium text-foreground">
        <span class="size-1.5 shrink-0 rounded-full bg-brand" aria-hidden="true"></span>
        {stageLabel}
      </span>
    {/if}
    <div class="ml-auto flex flex-wrap items-center justify-end gap-2">
      {#if canTailor}
        <Button
          variant={tailorIsPrimary ? 'primary' : 'outline'}
          size="sm"
          onclick={onTailor}
          disabled={tailoring}
          class={actionClass}
        >
          <SquarePen class="size-3.5 shrink-0" />
          <span class="truncate">{tailoring ? s.preparing : s.tailorCv}</span>
        </Button>
      {/if}
      {#if offersFollowUp}
        <Button
          variant="outline"
          size="sm"
          onclick={onFollowUp}
          class={cn(
            actionClass,
            'border-transparent bg-warning-muted text-warning-strong hover:bg-warning-muted hover:opacity-80',
          )}
        >
          <Send class="size-3.5 shrink-0" />
          <span class="truncate">{followedUp ? s.chaseAgain : s.followUp}</span>
        </Button>
      {/if}
    </div>
  </div>
{/if}
