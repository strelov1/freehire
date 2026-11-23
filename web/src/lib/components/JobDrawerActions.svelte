<script lang="ts">
  import { onDestroy } from 'svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { MessageSquare, Mic, Send, SquarePen } from '@lucide/svelte';
  import { Button, cn } from '#lib/ui/index.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { messages } from './JobDrawer.messages';

  let {
    stageLabel,
    canTailor,
    tailorIsPrimary,
    tailoring,
    onTailor,
    jobSlug,
    createInterviewSession,
    stage,
    offersFollowUp,
    followedUp,
    onFollowUp,
  }: {
    stageLabel: string | null;
    canTailor: boolean;
    tailorIsPrimary: boolean;
    tailoring: boolean;
    onTailor: () => void;
    jobSlug?: string | null;
    createInterviewSession: (preset: 'interview' | 'debrief', slug: string) => Promise<{ id: string }>;
    stage: string | null;
    offersFollowUp: boolean;
    followedUp: boolean;
    onFollowUp: () => void;
  } = $props();

  const s = $derived(t(messages, locale()));

  const canRehearse = $derived(!!jobSlug && (stage === 'screening' || stage === 'interview'));
  const canDebrief = $derived(!!jobSlug && ['interview', 'offer', 'accepted', 'rejected'].includes(stage ?? ''));
  let startingInterview = $state(false);
  let interviewError = $state(false);
  let disposed = false;
  onDestroy(() => {
    disposed = true;
  });

  async function startInterview(preset: 'interview' | 'debrief') {
    const slug = jobSlug;
    if (!slug || startingInterview) return;
    startingInterview = true;
    interviewError = false;
    try {
      const session = await createInterviewSession(preset, slug);
      if (!disposed && jobSlug === slug) await goto(resolve('/my/assistant/[[id]]', { id: session.id }));
    } catch {
      if (!disposed && jobSlug === slug) interviewError = true;
    } finally {
      startingInterview = false;
    }
  }

  // Actions keep their natural width, including when only Tailor CV is offered.
  const actionClass = 'h-9 min-w-0 gap-1.5 px-3 sm:h-8';
</script>

{#if stageLabel || canTailor || canRehearse || canDebrief || offersFollowUp}
  <div class="flex flex-wrap items-center gap-x-3 gap-y-2">
    {#if stageLabel}
      <span class="inline-flex items-center gap-1.5 text-xs font-medium text-foreground">
        <span class="size-1.5 shrink-0 rounded-full bg-brand" aria-hidden="true"></span>
        {stageLabel}
      </span>
    {/if}
    <div class="ml-auto flex flex-wrap items-center justify-end gap-2">
      {#if canRehearse}
        <Button
          variant="primary"
          size="sm"
          onclick={() => startInterview('interview')}
          disabled={startingInterview}
          class={actionClass}
        >
          <Mic class="size-3.5 shrink-0" aria-hidden="true" />
          <span class="truncate">{s.interview.rehearse}</span>
        </Button>
      {/if}
      {#if jobSlug && stage === 'interview'}
        <Button
          variant="outline"
          size="sm"
          href={`${resolve('/tools/interview-prep')}?job=${encodeURIComponent(jobSlug)}`}
          class={actionClass}>{s.interview.prepSheet}</Button
        >
      {/if}
      {#if canDebrief}
        <Button
          variant="outline"
          size="sm"
          onclick={() => startInterview('debrief')}
          disabled={startingInterview}
          class={actionClass}
        >
          <MessageSquare class="size-3.5 shrink-0" aria-hidden="true" />
          <span class="truncate">{s.interview.debrief}</span>
        </Button>
      {/if}
      {#if canTailor}
        <Button
          variant={tailorIsPrimary && !canRehearse ? 'primary' : 'outline'}
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

{#if interviewError}
  <p class="text-sm text-warning-strong" role="alert">{s.interview.startFailed}</p>
{/if}
