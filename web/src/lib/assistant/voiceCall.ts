// The parts of voice mode that are decisions rather than device/network handling:
// whether this browser can place the call at all, and how the Realtime API's
// streamed data-channel events turn into completed turns. VoiceCall.svelte owns the
// RTCPeerConnection, the microphone and the remote audio element; this owns the
// rules, so they can be tested without either.

/** How long one call may run before it ends itself.
 *
 *  Realtime audio bills per minute at several times dictation's per-minute rate (see
 *  the OpenSpec change's design.md), so a call left open is materially more expensive
 *  than a forgotten microphone. Fifteen minutes is well past a focused rehearsal of
 *  one interview round and well under anything worth leaving connected by accident. */
export const MAX_CALL_MS = 15 * 60 * 1000;

/** How long before the ceiling a warning appears. Resolved in design.md's Open
 *  Questions as warning-then-disconnect rather than a silent cutoff: a candidate
 *  mid-answer when a call hangs up with no notice has no way to tell a deliberate
 *  end from a dropped connection. */
export const END_WARNING_MS = 60 * 1000;

/** Strips a litellm-routing provider prefix (`openai/gpt-realtime-2.1` →
 *  `gpt-realtime-2.1`) from the model name the backend hands back with the minted
 *  token.
 *
 *  Two different systems read that name for two different reasons, and they
 *  disagree on its shape. The backend mints the client secret by POSTing to OUR
 *  litellm proxy, which needs the provider prefix to route the request to the right
 *  upstream account — that's why `REALTIME_MODEL` is configured as
 *  `openai/gpt-realtime-2.1` in the first place. But the browser then takes that
 *  same ephemeral token and negotiates the WebRTC call DIRECTLY against OpenAI's own
 *  `/v1/realtime/calls`, bypassing our proxy entirely — and OpenAI's real API has no
 *  concept of a litellm provider prefix; it 401s on a model id it does not
 *  recognise. This is the one place that mismatch has to be undone. */
export function openaiModelID(model: string): string {
  const slash = model.lastIndexOf('/');
  return slash === -1 ? model : model.slice(slash + 1);
}

/** The bits of `window` a call needs, as a shape rather than as the global, so the
 *  support check can be exercised for browsers this test run is not. */
export type VoiceCallEnv = {
  mediaDevices?: { getUserMedia?: unknown };
  RTCPeerConnection?: unknown;
};

/** Whether this browser can place a voice call.
 *
 *  Both halves are load-bearing, the same reasoning as dictation's canRecord:
 *  RTCPeerConnection is simply absent in some embedded browsers, and mediaDevices is
 *  absent outside a secure context. The interview session view renders no voice-mode
 *  control when this is false. */
export function canUseVoiceCall(env: VoiceCallEnv): boolean {
  return Boolean(env.RTCPeerConnection && env.mediaDevices?.getUserMedia);
}

/** One line of a voice-mode exchange, ready to be posted to the transcript. */
export type VoiceTurn = { role: 'user' | 'assistant'; content: string };

/** The pieces of a Realtime API server event this reducer reads. The full event
 *  carries more (session metadata, error detail); everything else passes through
 *  `applyRealtimeEvent` untouched. */
export type RealtimeServerEvent =
  | { type: 'conversation.item.input_audio_transcription.delta'; delta: string }
  | { type: 'conversation.item.input_audio_transcription.completed' }
  | { type: 'response.output_audio_transcript.delta'; delta: string }
  | { type: 'response.done' }
  | { type: string; [key: string]: unknown };

/** The accumulator `applyRealtimeEvent` folds events into: the in-progress text of
 *  whichever role is currently speaking, kept separate because the two streams
 *  interleave (the candidate's transcript can still be arriving after the agent's
 *  reply has started, if the API is still finishing transcription). */
export type VoiceCallState = { userText: string; agentText: string };

export function initVoiceCallState(): VoiceCallState {
  return { userText: '', agentText: '' };
}

/** Fold one server event into the accumulator. Returns the (possibly unchanged)
 *  state and, when a turn just completed, the turn to persist — mirroring the
 *  transcribe-then-hand-back shape `dictation.ts`'s appendTranscript uses, so the
 *  caller decides what to do with a completed turn rather than this reducer
 *  reaching for a network call itself. */
export function applyRealtimeEvent(
  state: VoiceCallState,
  event: RealtimeServerEvent,
): { state: VoiceCallState; completedTurn?: VoiceTurn } {
  switch (event.type) {
    case 'conversation.item.input_audio_transcription.delta':
      return { state: { ...state, userText: state.userText + event.delta } };
    case 'conversation.item.input_audio_transcription.completed': {
      const content = state.userText.trim();
      const next = { ...state, userText: '' };
      if (!content) return { state: next };
      return { state: next, completedTurn: { role: 'user', content } };
    }
    case 'response.output_audio_transcript.delta':
      return { state: { ...state, agentText: state.agentText + event.delta } };
    case 'response.done': {
      const content = state.agentText.trim();
      const next = { ...state, agentText: '' };
      if (!content) return { state: next };
      return { state: next, completedTurn: { role: 'assistant', content } };
    }
    default:
      return { state };
  }
}
