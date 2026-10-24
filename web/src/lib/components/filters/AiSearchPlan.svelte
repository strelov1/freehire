<script lang="ts">
  import { MessageSquare, Play } from '@lucide/svelte';
  import { Button, Card, Chip } from '#lib/ui/index.js';
  import { planPreview } from '#lib/aiFilter.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, t } from '#lib/i18n/t.js';
  import type { Interpretation } from '#lib/types.js';
  import { messages } from './AiSearchPlan.messages';

  // One plan the agent proposed, as it appears in the chat: the model's sentence, the
  // values it will apply, and whatever it could not place. The chips are the guard the
  // summary needs — a sentence that names a place the dictionaries dropped reads as a
  // promise, and the chips are what will actually be written to the job list. Unplaced
  // words are listed for the same reason: a drop nobody is told about is
  // indistinguishable from a value that was applied.
  //
  // `actionable` puts the Run / Keep chatting card under it. The host passes it for the
  // LATEST plan on the plan step, whatever has been said since — an agent prompt after
  // Keep chatting does not make the plan any less runnable.
  let {
    plan,
    actionable,
    compact = false,
    onRun,
    onKeepChatting,
  }: {
    plan: Interpretation;
    actionable: boolean;
    compact?: boolean;
    onRun: () => void;
    onKeepChatting: () => void;
  } = $props();

  const s = $derived(t(messages, locale()));
  const preview = $derived(planPreview(plan));
</script>

<div class="space-y-3 text-sm leading-relaxed">
  <p>{s.intro}</p>
  <p class="font-medium">{plan.summary}</p>

  {#if preview.length}
    <div class="flex flex-col gap-2">
      {#each preview as group (group.label)}
        <div>
          <p class="text-xs font-medium text-muted-foreground">{group.label}</p>
          <div class="mt-1 flex flex-wrap gap-1.5">
            {#each group.chips as chip (chip.key)}
              <Chip variant={chip.exclude ? 'destructive' : 'brand'} class={chip.exclude ? 'line-through' : ''}>
                {chip.text}
              </Chip>
            {/each}
          </div>
        </div>
      {/each}
    </div>
  {/if}

  {#if plan.unresolved?.length}
    <p class="text-xs text-muted-foreground">{format(s.notRecognised, { values: plan.unresolved.join(', ') })}</p>
  {/if}

  {#if actionable}
    <p>{s.askRun}</p>
    <Card class="p-4">
      <p class="font-medium">{s.cardTitle}</p>
      <p class="mt-1 text-muted-foreground">{s.cardBody}</p>
      <div class="mt-3 grid gap-2 {compact ? '' : 'sm:grid-cols-2'}">
        <Button variant="primary" size="lg" onclick={onRun}><Play class="size-4" /> {s.run}</Button>
        <Button variant="outline" size="lg" onclick={onKeepChatting}><MessageSquare class="size-4" /> {s.keepChatting}</Button>
      </div>
    </Card>
  {/if}
</div>
