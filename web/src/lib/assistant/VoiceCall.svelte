<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import { Loader2, PhoneOff } from '@lucide/svelte';
  import {
    mintVoiceToken,
    appendVoiceTurn,
    VoiceModeUnavailable,
    SessionNotFound,
    type VoiceToken,
  } from '$lib/assistant/api';
  import {
    applyRealtimeEvent,
    initVoiceCallState,
    MAX_CALL_MS,
    END_WARNING_MS,
    type RealtimeServerEvent,
  } from '$lib/assistant/voiceCall';

  // The voice-mode overlay: mints one call's credential, connects directly to OpenAI
  // over WebRTC, and appends each completed turn to the session as it finishes. Audio
  // never transits our backend — only the mint and the per-turn appends do.
  //
  // Everything device/network-specific lives here; the accumulate-until-a-turn-
  // completes logic lives in voiceCall.ts so it is testable without a microphone,
  // the same split VoiceInput.svelte and dictation.ts already use.
  let {
    sessionId,
    onClose,
    onUnavailable,
  }: {
    sessionId: string;
    /** The call ended, one way or another — connection failure, the caller hung up, or
     *  the duration ceiling. The host reloads the transcript and returns to text mode. */
    onClose: () => void;
    /** This deployment has no Realtime gateway. The host stops offering voice mode —
     *  the feature is absent here, not broken. */
    onUnavailable: () => void;
  } = $props();

  type Phase = 'connecting' | 'active' | 'error';
  let phase = $state<Phase>('connecting');
  let errorMessage = $state<string | null>(null);
  let userLine = $state('');
  let agentLine = $state('');
  let endingSoon = $state(false);

  let pc: RTCPeerConnection | null = null;
  let dc: RTCDataChannel | null = null;
  let micStream: MediaStream | null = null;
  let remoteAudioEl: HTMLAudioElement | null = null;
  let callState = initVoiceCallState();
  let ceiling: ReturnType<typeof setTimeout> | null = null;
  let warnTimer: ReturnType<typeof setTimeout> | null = null;
  // Guards against a stray event reaching the UI after the call has already been torn
  // down — onDestroy can race a still-in-flight connect().
  let live = true;

  async function connect() {
    let token: VoiceToken;
    try {
      token = await mintVoiceToken(sessionId);
    } catch (err) {
      if (err instanceof VoiceModeUnavailable) {
        onUnavailable();
        return;
      }
      fail(err instanceof SessionNotFound ? 'This conversation could not be found.' : messageOf(err));
      return;
    }
    if (!live) return;

    try {
      micStream = await navigator.mediaDevices.getUserMedia({ audio: true });
    } catch {
      fail('The microphone is unavailable — check this site’s permission for it.');
      return;
    }
    if (!live) {
      micStream.getTracks().forEach((t) => t.stop());
      return;
    }

    try {
      pc = new RTCPeerConnection();
      pc.ontrack = (e) => {
        if (remoteAudioEl) remoteAudioEl.srcObject = e.streams[0] ?? null;
      };
      // A connection that dies mid-call (network drop, ICE failure, the ephemeral
      // secret's TTL expiring) must not leave the UI stuck on "active" with the mic
      // still open and no way out — the same failure class VoiceInput.svelte's
      // recorder.onerror guards for on the dictation side.
      pc.onconnectionstatechange = () => {
        if (pc && (pc.connectionState === 'failed' || pc.connectionState === 'closed')) {
          fail('The call disconnected unexpectedly.');
        }
      };
      for (const track of micStream.getTracks()) pc.addTrack(track, micStream);

      dc = pc.createDataChannel('oai-events');
      dc.addEventListener('open', () => {
        if (live) phase = 'active';
      });
      dc.addEventListener('close', () => fail('The call disconnected unexpectedly.'));
      dc.addEventListener('message', (e) => handleServerEvent(e.data));

      const offer = await pc.createOffer();
      await pc.setLocalDescription(offer);

      const sdpResp = await fetch(`https://api.openai.com/v1/realtime/calls?model=${encodeURIComponent(token.model)}`, {
        method: 'POST',
        body: offer.sdp,
        headers: { Authorization: `Bearer ${token.value}`, 'Content-Type': 'application/sdp' },
      });
      if (!sdpResp.ok) throw new Error(`Could not connect the call (${sdpResp.status}).`);
      const answerSdp = await sdpResp.text();
      if (!live) return;
      await pc.setRemoteDescription({ type: 'answer', sdp: answerSdp });
    } catch (err) {
      fail(messageOf(err));
      return;
    }

    // Realtime audio bills per minute at several times dictation's rate, so a call
    // left open is materially more expensive than a forgotten microphone — this is
    // the backstop past the per-hour mint limit the server already holds. The warning
    // fires first so a candidate mid-answer can tell a deliberate end from a drop.
    warnTimer = setTimeout(() => {
      if (live) endingSoon = true;
    }, MAX_CALL_MS - END_WARNING_MS);
    ceiling = setTimeout(() => end(), MAX_CALL_MS);
  }

  function handleServerEvent(raw: string) {
    if (!live) return;
    let event: RealtimeServerEvent;
    try {
      event = JSON.parse(raw) as RealtimeServerEvent;
    } catch {
      return;
    }
    const result = applyRealtimeEvent(callState, event);
    callState = result.state;
    userLine = callState.userText;
    agentLine = callState.agentText;
    if (result.completedTurn) {
      void persistTurn(result.completedTurn.role, result.completedTurn.content);
    }
  }

  /** Append one turn, with one retry. A transient failure here must not end an
   *  otherwise-healthy call — but silently dropping it on the first try would still
   *  lose a completed turn the spec says must survive, so one retry stands between
   *  "best-effort" and "actually usually works". */
  async function persistTurn(role: 'user' | 'assistant', content: string) {
    try {
      await appendVoiceTurn(sessionId, role, content);
    } catch {
      try {
        await appendVoiceTurn(sessionId, role, content);
      } catch (err) {
        console.error('voice mode: could not persist a turn', err);
      }
    }
  }

  function messageOf(err: unknown): string {
    return err instanceof Error ? err.message : 'Could not start voice mode.';
  }

  function fail(message: string) {
    // Guards against a second failure signal (e.g. onconnectionstatechange AND the
    // data channel closing) re-running teardown and overwriting an already-shown
    // message.
    if (!live || phase === 'error') return;
    errorMessage = message;
    phase = 'error';
    teardown();
  }

  function end() {
    teardown();
    onClose();
  }

  function teardown() {
    if (ceiling !== null) clearTimeout(ceiling);
    ceiling = null;
    if (warnTimer !== null) clearTimeout(warnTimer);
    warnTimer = null;
    if (remoteAudioEl) remoteAudioEl.srcObject = null;
    dc?.close();
    dc = null;
    pc?.close();
    pc = null;
    micStream?.getTracks().forEach((t) => t.stop());
    micStream = null;
  }

  onMount(() => {
    void connect();
  });
  onDestroy(() => {
    live = false;
    teardown();
  });
</script>

<div class="flex flex-col items-center gap-4 rounded-2xl border border-border bg-card p-6">
  <audio bind:this={remoteAudioEl} autoplay class="hidden"></audio>

  {#if phase === 'connecting'}
    <div class="flex items-center gap-2 text-sm text-muted-foreground">
      <Loader2 class="size-4 animate-spin" />
      Connecting…
    </div>
  {:else if phase === 'error'}
    <p class="text-sm text-destructive">{errorMessage}</p>
    <button
      type="button"
      onclick={onClose}
      class="rounded-full border border-border px-4 py-2 text-sm font-medium text-foreground hover:bg-muted"
    >
      Close
    </button>
  {:else}
    {#if endingSoon}
      <p class="text-xs font-medium text-amber-600 dark:text-amber-400">
        This call ends in about a minute.
      </p>
    {/if}
    <div class="flex min-h-16 w-full flex-col gap-1 text-sm" aria-live="polite">
      {#if agentLine}
        <p class="text-foreground"><span class="text-muted-foreground">Interviewer: </span>{agentLine}</p>
      {/if}
      {#if userLine}
        <p class="text-brand"><span class="text-muted-foreground">You: </span>{userLine}</p>
      {/if}
    </div>
    <button
      type="button"
      onclick={end}
      aria-label="End call"
      title="End call"
      class="flex size-12 items-center justify-center rounded-full bg-destructive text-destructive-foreground transition-opacity hover:opacity-90"
    >
      <PhoneOff class="size-5" />
    </button>
  {/if}
</div>
