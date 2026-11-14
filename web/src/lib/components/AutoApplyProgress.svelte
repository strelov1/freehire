<script lang="ts">
  import { Check, X } from '@lucide/svelte';
  import { cn } from '#lib/ui/index.js';
  import type { AutoApplyProgressStep, AutoApplyProgressStepId } from '#lib/autoApplyProgress.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { messages } from './JobDrawer.messages';

  // An automatic application's steps as a numbered stepper, in JobDrawer's auto-apply tab.
  let { steps }: { steps: AutoApplyProgressStep[] } = $props();

  const s = $derived(t(messages, locale()));
  const labels = $derived<Record<AutoApplyProgressStepId, string>>({
    tailoring: s.progressSteps.tailoring,
    review: s.progressSteps.review,
    submitted: s.progressSteps.submitted,
  });
</script>

<ol class="flex items-center" aria-label={s.autoApplyProgressAria}>
  {#each steps as step, i (step.id)}
    <li class="flex flex-1 items-center last:flex-none">
      <div class="flex flex-col items-center gap-1">
        <span
          class={cn(
            'flex size-6 shrink-0 items-center justify-center rounded-full text-xs font-medium',
            step.state === 'done' && 'bg-brand text-brand-foreground',
            step.state === 'active' && 'bg-brand-muted text-brand-strong ring-2 ring-brand-ring',
            step.state === 'queued' && 'bg-muted text-muted-foreground ring-2 ring-border',
            step.state === 'error' && 'bg-destructive/15 text-destructive-strong',
            step.state === 'pending' && 'bg-muted text-muted-foreground',
          )}
        >
          {#if step.state === 'done'}
            <Check class="size-3.5" />
          {:else if step.state === 'error'}
            <X class="size-3.5" />
          {:else}
            {i + 1}
          {/if}
        </span>
        <span class="text-xs font-medium whitespace-nowrap">{labels[step.id]}</span>
      </div>
      {#if i < steps.length - 1}
        <span class="mx-1 h-px flex-1 bg-border" aria-hidden="true"></span>
      {/if}
    </li>
  {/each}
</ol>
