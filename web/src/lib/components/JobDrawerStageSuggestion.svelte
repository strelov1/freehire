<script lang="ts">
  import { statusLabel } from '#lib/emailStatus.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, t } from '#lib/i18n/t.js';
  import { humanizeStage } from '#lib/stages.js';
  import type { StageSuggestion } from '#lib/types.js';
  import { Button } from '#lib/ui/index.js';
  import { messages } from './JobDrawer.messages';

  // The mail says one thing, the stage says another, and nothing moved it. That is the rule
  // working — mail never settles an application — so the resolution is offered rather than
  // applied, and it goes through the ordinary stage change so the ledger records the
  // candidate as its source.
  let {
    suggestion,
    currentStage,
    onmove,
    ondismiss,
  }: {
    suggestion: StageSuggestion;
    currentStage: string | null | undefined;
    onmove: (stage: string) => void;
    ondismiss: () => void;
  } = $props();

  const s = $derived(t(messages, locale()));
</script>

<div class="flex flex-wrap items-center gap-2 rounded-md border border-warning/50 bg-warning-muted/40 px-3 py-2">
  <span class="min-w-0 flex-1 text-sm">
    {format(s.looksLike, {
      signal: statusLabel(suggestion.signal, locale()).toLowerCase(),
      stage: currentStage ? humanizeStage(currentStage, locale()) : s.stageUnset,
    })}
  </span>
  <Button size="sm" variant="outline" class="shrink-0" onclick={() => onmove(suggestion.stage)}>
    {format(s.moveTo, { stage: humanizeStage(suggestion.stage, locale()) })}
  </Button>
  <button
    type="button"
    class="shrink-0 text-xs text-muted-foreground underline-offset-2 hover:underline"
    onclick={ondismiss}
  >
    {s.dismiss}
  </button>
</div>
