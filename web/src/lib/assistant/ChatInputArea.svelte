<script lang="ts">
  import { onMount } from 'svelte';
  import { AudioLines, Phone } from '@lucide/svelte';
  import { VOICE_CALL_ENABLED } from './audioAvailability';
  import { canUseVoiceCall } from './voiceCall';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import type { Attachment } from './client';
  import type { ChatMessage } from './chat';
  import Composer from './Composer.svelte';
  import InterviewPlayback from './InterviewPlayback.svelte';
  import { transcribeInterviewAudio } from './speech';
  import { messages } from './AssistantChat.messages';

  // AssistantChat's input area: the composer, or a hands-free voice call in its place while
  // `voiceCallOpen`, offered on an `interview` session where this browser can hold one.
  let {
    draft = $bindable(),
    attachments = $bindable(),
    voiceCallOpen = $bindable(),
    sessionId,
    queue,
    turnActive,
    support,
    disabled,
    interview,
    elevenLabsVoice,
    transcript,
    onSubmit,
    onRemoveQueued,
    onCancel,
    onClose,
  }: {
    draft: string;
    attachments: Attachment[];
    voiceCallOpen: boolean;
    sessionId: string | null;
    queue: { id: string; text: string; attachments: Attachment[] }[];
    turnActive: boolean;
    support: boolean;
    disabled: boolean;
    interview: boolean;
    elevenLabsVoice: boolean;
    transcript: ChatMessage[];
    onSubmit: (text: string, docs: Attachment[]) => void;
    onRemoveQueued: (id: string) => void;
    onCancel: () => void;
    onClose: () => void;
  } = $props();

  const s = $derived(t(messages, locale()));

  // Voice mode: a hands-free call on an `interview` session, replacing the composer
  // for its duration. `voiceModeOff` latches once the server reports no Realtime
  // gateway (501) — same reasoning as Composer's `dictationOff` — so the trigger does
  // not keep offering a call that can only fail. `voiceSupported` starts false and is
  // decided after mount, the same reasoning VoiceInput.svelte's `supported` gives:
  // reading navigator during SSR would disagree with the client on the first paint.
  // It also carries VOICE_CALL_ENABLED — see audioAvailability.ts.
  let elevenLabsSession = $state<string | null>(null);
  const elevenLabsSelected = $derived(Boolean(sessionId) && sessionId === elevenLabsSession && elevenLabsVoice);
  const voiceTranscriber = $derived.by(() => {
    const id = sessionId;
    if (!id || !elevenLabsSelected) return undefined;
    return (audio: Blob, filename: string) => transcribeInterviewAudio(id, audio, filename);
  });

  const interviewReplyKey = $derived.by(() => {
    const reply = transcript.at(-1);
    return reply?.role === 'assistant' && reply.text.trim() && !reply.streaming && !reply.errored
      ? transcript.length
      : 0;
  });

  let voiceModeOff = $state(false);
  let voiceSupported = $state(false);
  const canCall = $derived(interview && voiceSupported && !voiceModeOff);
  onMount(() => {
    voiceSupported =
      VOICE_CALL_ENABLED &&
      canUseVoiceCall({
        mediaDevices: navigator.mediaDevices,
        RTCPeerConnection: typeof RTCPeerConnection === 'undefined' ? undefined : RTCPeerConnection,
      });
  });
  function onUnavailable() {
    voiceModeOff = true;
    onClose();
  }
</script>

{#if voiceCallOpen && sessionId}
  <!-- Keyed on activeId: switching sessions while a call is open must destroy
     this instance (its onDestroy ends the call cleanly) rather than reuse it
     with a new sessionId prop, which would silently post the OLD call's turns
     into the NEW session's transcript. -->
  {#key sessionId}
    <div class="border-t border-border p-3">
      <div class="mx-auto w-full max-w-3xl">
        {#await import('./VoiceCall.svelte') then { default: VoiceCall }}
          <VoiceCall {sessionId} {onClose} {onUnavailable} />
        {:catch}
          <p role="alert" class="text-sm text-destructive">{s.voiceCallLoadFailed}</p>
          <button
            type="button"
            class="text-sm text-muted-foreground underline-offset-2 hover:underline"
            onclick={() => (voiceCallOpen = false)}>{s.returnToText}</button
          >
        {/await}
      </div>
    </div>
  {/key}
{:else}
  {#if elevenLabsSelected && sessionId}
    {#key sessionId}
      <InterviewPlayback {sessionId} replyKey={interviewReplyKey} />
    {/key}
  {/if}
  <Composer
    bind:draft
    bind:attachments
    {queue}
    {turnActive}
    attachable={!support}
    dictatable={!support}
    {voiceTranscriber}
    {disabled}
    {onSubmit}
    {onRemoveQueued}
    {onCancel}
  />
  {#if (canCall || elevenLabsVoice) && sessionId}
    <div class="flex flex-wrap justify-center gap-2 border-t border-border/60 py-2">
      {#if elevenLabsVoice}
        <button
          type="button"
          aria-pressed={elevenLabsSelected}
          disabled={disabled || turnActive}
          onclick={() => (elevenLabsSession = elevenLabsSelected ? null : sessionId)}
          class="flex items-center gap-1.5 rounded-full px-3 py-1.5 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground aria-pressed:bg-accent aria-pressed:text-brand-strong disabled:opacity-50"
        >
          <AudioLines class="size-3.5" aria-hidden="true" />
          {s.elevenLabsVoice}
        </button>
      {/if}
      {#if canCall}
        <button
          type="button"
          onclick={() => {
            elevenLabsSession = null;
            voiceCallOpen = true;
          }}
          disabled={disabled || turnActive}
          class="flex items-center gap-1.5 rounded-full px-3 py-1.5 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:cursor-not-allowed disabled:opacity-50"
        >
          <Phone class="size-3.5" />
          {s.voiceModeButton}
        </button>
      {/if}
    </div>
  {/if}
{/if}
