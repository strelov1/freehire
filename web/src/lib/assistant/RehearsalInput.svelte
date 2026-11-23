<script lang="ts">
  import type { Snippet } from 'svelte';
  import { finishInterview, repeatInterviewQuestion, type InterviewReport } from './api';
  import type { ChatMessage } from './chat';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { messages } from './AssistantChat.messages';

  let {
    id,
    initialReport,
    transcript,
    callOpen,
    ready,
    onReplay,
    children,
  }: {
    id: string;
    initialReport?: InterviewReport;
    transcript: ChatMessage[];
    callOpen: boolean;
    ready: boolean;
    onReplay: (id: string) => Promise<void>;
    children: Snippet<[boolean]>;
  } = $props();

  const s = $derived(t(messages, locale()));
  let completedReport = $state<InterviewReport>();
  const report = $derived(completedReport ?? initialReport);
  let busy = $state(false);
  let error = $state('');
  const hasAnswer = $derived.by(() => {
    const firstReply = transcript.findIndex((message) => message.role === 'assistant' && message.text.trim());
    return (
      firstReply >= 0 &&
      transcript.some((message, index) => index > firstReply && message.role === 'user' && message.text.trim())
    );
  });

  async function finish() {
    if (busy || !ready) return;
    busy = true;
    error = '';
    try {
      completedReport = await finishInterview(id);
    } catch {
      error = s.interviewReportError;
    } finally {
      busy = false;
    }
  }

  async function repeat() {
    if (busy) return;
    busy = true;
    error = '';
    const currentID = id;
    try {
      const result = await repeatInterviewQuestion(currentID);
      await onReplay(result.session_id);
    } catch {
      error = s.interviewRepeatError;
    } finally {
      busy = false;
    }
  }
</script>

{#if report}
  <section class="max-h-80 shrink-0 overflow-y-auto border-t border-border px-4 py-3">
    <div class="mx-auto grid max-w-3xl gap-2">
      <h2 class="text-base font-semibold">{s.interviewReport}</h2>
      <dl class="grid gap-2 text-sm">
        <div>
          <dt class="font-medium">{s.interviewStrength}</dt>
          <dd>{report.strength}</dd>
        </div>
        <div>
          <dt class="font-medium">{s.interviewImprovement}</dt>
          <dd>{report.improvement}</dd>
        </div>
        <div>
          <dt class="font-medium">{s.interviewQuote}</dt>
          <dd>
            <blockquote class="border-l-2 border-border pl-2 text-muted-foreground">
              {report.quote}
            </blockquote>
          </dd>
        </div>
        <div>
          <dt class="font-medium">{s.interviewQuestion}</dt>
          <dd class="whitespace-pre-line">{report.question}</dd>
        </div>
      </dl>
      <button
        type="button"
        class="w-fit rounded-md border border-border px-3 py-1.5 text-sm font-medium hover:bg-muted disabled:opacity-50"
        disabled={busy}
        onclick={repeat}
      >
        {s.repeatInterviewQuestion}
      </button>
    </div>
  </section>
{:else}
  {@render children(busy)}
  {#if hasAnswer && !callOpen}
    <div class="flex justify-center pb-2">
      <button
        type="button"
        class="rounded-md px-3 py-1.5 text-sm font-medium text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-50"
        disabled={!ready || busy}
        onclick={finish}
      >
        {busy ? s.loadingInterviewReport : s.finishInterview}
      </button>
    </div>
  {/if}
{/if}
{#if error}
  <p role="alert" class="px-4 pb-2 text-center text-sm text-destructive">{error}</p>
{/if}
