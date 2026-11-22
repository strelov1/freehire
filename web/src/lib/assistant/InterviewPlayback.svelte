<script lang="ts">
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { interviewSpeech } from './api';
  import { messages } from './AssistantChat.messages';

  let { sessionId, replyKey }: { sessionId: string; replyKey: number } = $props();

  const s = $derived(t(messages, locale()));
  let player = $state<HTMLAudioElement>();
  let playbackState = $state<'idle' | 'loading' | 'ready' | 'error'>('idle');
  let retry = $state(0);

  $effect(() => {
    const audio = player;
    const id = sessionId;
    const key = replyKey;
    void retry;
    if (!audio || key === 0) return;

    const request = new AbortController();
    let url = '';
    playbackState = 'loading';

    void interviewSpeech(id, request.signal)
      .then((blob) => {
        if (request.signal.aborted) return;
        url = URL.createObjectURL(blob);
        audio.src = url;
        playbackState = 'ready';
        void audio.play().catch(() => {});
      })
      .catch(() => {
        if (!request.signal.aborted) playbackState = 'error';
      });

    return () => {
      request.abort();
      audio.pause();
      audio.removeAttribute('src');
      if (url) URL.revokeObjectURL(url);
      playbackState = 'idle';
    };
  });
</script>

<div class="flex flex-wrap items-center justify-center gap-2 text-sm text-muted-foreground">
  <audio bind:this={player} controls hidden={playbackState !== 'ready'} aria-label={s.interviewerPlayback}></audio>
  {#if playbackState === 'loading'}
    <p role="status">{s.preparingInterviewAudio}</p>
  {:else if playbackState === 'error'}
    <p role="alert">{s.interviewAudioFailed}</p>
    <button type="button" class="underline hover:text-foreground" onclick={() => retry++}>{s.retry}</button>
  {/if}
</div>
