<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import { Check, Loader2, Mic, X } from '@lucide/svelte';
  import { canRecord, pickContainer, MAX_RECORDING_MS } from '$lib/assistant/dictation';
  import { transcribe, TranscriptionUnavailable } from '$lib/assistant/speech';

  // Dictation for the composer: record, transcribe, hand the words back. It never
  // sends anything — a transcription is a guess about what was said, and a wrong guess
  // sent on the caller's behalf is a message they cannot unsend.
  //
  // The rules live in dictation.ts so they can be tested without a microphone. What is
  // here is the part that cannot be: the stream, the recorder, the clock, the wake lock,
  // and making sure every one of them is released on every way out.
  let {
    disabled = false,
    onTranscript,
    onError,
    onUnavailable,
  }: {
    disabled?: boolean;
    /** The words. Appended to the draft by the host; never sent. */
    onTranscript: (text: string) => void;
    /** Something the caller can act on: a denied permission, a failed upload. */
    onError: (message: string) => void;
    /** This deployment has no speech gateway. The host stops rendering the control —
     *  the feature is absent here, not broken, and an control that can only fail
     *  teaches people to stop reaching for it. */
    onUnavailable: () => void;
  } = $props();

  type Phase = 'idle' | 'recording' | 'transcribing';

  let phase = $state<Phase>('idle');
  let elapsedMs = $state(0);

  // Device handles and timers. Deliberately NOT $state: nothing renders from them, and
  // making a MediaStream deeply reactive would proxy an object the browser owns.
  let stream: MediaStream | null = null;
  let recorder: MediaRecorder | null = null;
  let chunks: Blob[] = [];
  let wakeLock: WakeLockSentinel | null = null;
  let ticker: ReturnType<typeof setInterval> | null = null;
  let ceiling: ReturnType<typeof setTimeout> | null = null;
  // Whether the recording that is stopping should be transcribed. `stop` fires the same
  // way for a confirmation, a cancellation and the ceiling, so the intent has to be
  // recorded before the recorder is asked to stop rather than inferred afterwards.
  let keeping = false;

  // Decided after mount, not at init. The server renders this component too, and a
  // value read from `navigator` would be false there and true here — a hydration
  // mismatch on the first paint. Starting false means the control appears a tick
  // late, which nobody sees, instead of the markup disagreeing with itself.
  let supported = $state(false);

  onMount(() => {
    supported = canRecord({
      mediaDevices: navigator.mediaDevices,
      MediaRecorder: typeof MediaRecorder === 'undefined' ? undefined : MediaRecorder,
    });
  });

  const elapsed = $derived(formatElapsed(elapsedMs));

  function formatElapsed(ms: number): string {
    const total = Math.floor(ms / 1000);
    return `${Math.floor(total / 60)}:${String(total % 60).padStart(2, '0')}`;
  }

  /** Hold the screen awake while recording. Without it a phone locks mid-sentence and
   *  the capture stops. Absent in some browsers, which is not an error — dictation
   *  works, it just does not survive a locked screen there. */
  async function holdScreen() {
    try {
      wakeLock = (await navigator.wakeLock?.request('screen')) ?? null;
    } catch {
      wakeLock = null;
    }
  }

  /** Release everything this component took from the browser. Safe to call twice, and
   *  called on every exit: confirm, cancel, ceiling, failure, and unmount. */
  function release() {
    if (ticker !== null) clearInterval(ticker);
    if (ceiling !== null) clearTimeout(ceiling);
    ticker = null;
    ceiling = null;
    stream?.getTracks().forEach((track) => track.stop());
    stream = null;
    void wakeLock?.release().catch(() => {});
    wakeLock = null;
  }

  async function start() {
    if (disabled || phase !== 'idle') return;
    const container = pickContainer((type) => MediaRecorder.isTypeSupported(type));
    if (!container) {
      onError('This browser cannot record audio in a format we can transcribe.');
      return;
    }
    try {
      stream = await navigator.mediaDevices.getUserMedia({
        audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true },
      });
    } catch {
      onError('The microphone is unavailable — check this site’s permission for it.');
      return;
    }

    chunks = [];
    keeping = false;
    recorder = new MediaRecorder(stream, { mimeType: container.mimeType });
    recorder.ondataavailable = (event) => {
      if (event.data.size > 0) chunks.push(event.data);
    };
    // Without this a recorder that fails mid-capture leaves the component stuck in
    // `recording` with the microphone still open and no control that ends it.
    recorder.onerror = () => {
      keeping = false;
      release();
      recorder = null;
      chunks = [];
      phase = 'idle';
      onError('The recording stopped unexpectedly.');
    };
    recorder.onstop = () => {
      release();
      const captured = chunks;
      chunks = [];
      recorder = null;
      if (keeping) void send(captured, container);
      else phase = 'idle';
    };
    recorder.start();
    phase = 'recording';

    const startedAt = Date.now();
    elapsedMs = 0;
    ticker = setInterval(() => (elapsedMs = Date.now() - startedAt), 250);
    // The ceiling is not a nicety: transcription is billed per minute against an
    // assistant nobody meters, and the server refuses an upload past its cap. Stopping
    // ourselves keeps a forgotten microphone from becoming either.
    ceiling = setTimeout(() => stop(true), MAX_RECORDING_MS);
    void holdScreen();
  }

  /** End the recording. `keep` decides whether it is transcribed or discarded. */
  function stop(keep: boolean) {
    if (phase !== 'recording') return;
    keeping = keep;
    // Ask the recorder to stop rather than tearing down here: `onstop` is where the
    // captured chunks arrive, and releasing the stream first would lose the tail.
    recorder?.stop();
  }

  async function send(captured: Blob[], container: { mimeType: string; filename: string }) {
    phase = 'transcribing';
    try {
      const text = await transcribe(new Blob(captured, { type: container.mimeType }), container.filename);
      // Silence transcribes to an empty string. Handing it on is harmless — the host
      // appends nothing — and reporting it as a failure would be a lie.
      onTranscript(text);
    } catch (err) {
      if (err instanceof TranscriptionUnavailable) onUnavailable();
      else onError(err instanceof Error ? err.message : 'Could not transcribe the recording.');
    } finally {
      phase = 'idle';
    }
  }

  onDestroy(() => {
    // Cancel rather than confirm: a component going away is not a decision to send.
    keeping = false;
    recorder?.stop();
    release();
  });
</script>

<!-- Escape abandons a recording, matching every other dismissable thing in the app.
     Bound to the window rather than to the button so it works while focus is on the
     textarea, which is where it will be. It sits at the top level because Svelte
     requires it there; the guard is the phase, which is only ever `recording` when
     this browser could record in the first place. -->
<svelte:window
  onkeydown={(e) => {
    if (e.key === 'Escape' && phase === 'recording') {
      e.preventDefault();
      stop(false);
    }
  }}
/>

{#if supported}
  {#if phase === 'recording'}
    <div
      class="flex shrink-0 items-center gap-1 rounded-full bg-destructive/10 px-1.5 py-1"
      role="status"
      aria-label="Recording"
    >
      <button
        type="button"
        onclick={() => stop(false)}
        aria-label="Discard recording"
        title="Discard (Esc)"
        class="flex size-7 items-center justify-center rounded-full text-muted-foreground transition-colors hover:text-foreground"
      >
        <X class="size-4" />
      </button>
      <span class="pulse size-2 shrink-0 rounded-full bg-destructive"></span>
      <span class="min-w-[2.5rem] text-center font-mono text-xs tabular-nums text-muted-foreground">
        {elapsed}
      </span>
      <button
        type="button"
        onclick={() => stop(true)}
        aria-label="Use recording"
        title="Use recording"
        class="flex size-7 items-center justify-center rounded-full bg-foreground text-background transition-opacity hover:opacity-90"
      >
        <Check class="size-4" />
      </button>
    </div>
  {:else}
    <button
      type="button"
      onclick={start}
      disabled={disabled || phase === 'transcribing'}
      aria-label={phase === 'transcribing' ? 'Transcribing' : 'Dictate a message'}
      aria-busy={phase === 'transcribing'}
      title="Dictate a message"
      class="flex size-9 shrink-0 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:cursor-not-allowed disabled:opacity-50"
    >
      {#if phase === 'transcribing'}
        <Loader2 class="size-4 animate-spin" />
      {:else}
        <Mic class="size-4" />
      {/if}
    </button>
  {/if}
{/if}

<style>
  .pulse {
    animation: pulse-dot 1.4s ease-in-out infinite;
  }
  @keyframes pulse-dot {
    0%,
    100% {
      opacity: 0.35;
    }
    50% {
      opacity: 1;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .pulse {
      animation: none;
      opacity: 1;
    }
  }
</style>
