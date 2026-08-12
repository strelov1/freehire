import { describe, it, expect } from 'vitest';
import { applyRealtimeEvent, canUseVoiceCall, initVoiceCallState } from './voiceCall';

describe('canUseVoiceCall', () => {
  it('is true only when both APIs are present', () => {
    expect(
      canUseVoiceCall({ mediaDevices: { getUserMedia: () => {} }, RTCPeerConnection: {} }),
    ).toBe(true);
  });

  it('is false without RTCPeerConnection', () => {
    expect(canUseVoiceCall({ mediaDevices: { getUserMedia: () => {} } })).toBe(false);
  });

  it('is false without getUserMedia, which is what an insecure context looks like', () => {
    expect(canUseVoiceCall({ RTCPeerConnection: {} })).toBe(false);
    expect(canUseVoiceCall({ mediaDevices: {}, RTCPeerConnection: {} })).toBe(false);
  });

  it('is false in a bare environment', () => {
    expect(canUseVoiceCall({})).toBe(false);
  });
});

describe('applyRealtimeEvent', () => {
  it('accumulates candidate transcript deltas without emitting a turn', () => {
    let state = initVoiceCallState();
    const first = applyRealtimeEvent(state, {
      type: 'conversation.item.input_audio_transcription.delta',
      delta: 'Let',
    });
    state = first.state;
    expect(first.completedTurn).toBeUndefined();

    const second = applyRealtimeEvent(state, {
      type: 'conversation.item.input_audio_transcription.delta',
      delta: "'s do the technical round.",
    });
    expect(second.completedTurn).toBeUndefined();
    expect(second.state.userText).toBe("Let's do the technical round.");
  });

  it('emits a completed user turn and clears the accumulator', () => {
    const mid = applyRealtimeEvent(initVoiceCallState(), {
      type: 'conversation.item.input_audio_transcription.delta',
      delta: 'Kafka in production.',
    });
    const done = applyRealtimeEvent(mid.state, {
      type: 'conversation.item.input_audio_transcription.completed',
    });
    expect(done.completedTurn).toEqual({ role: 'user', content: 'Kafka in production.' });
    expect(done.state.userText).toBe('');
  });

  it('emits a completed agent turn on response.done, separately from the user text', () => {
    let state = initVoiceCallState();
    state = applyRealtimeEvent(state, {
      type: 'conversation.item.input_audio_transcription.delta',
      delta: 'still arriving',
    }).state;
    state = applyRealtimeEvent(state, {
      type: 'response.output_audio_transcript.delta',
      delta: 'Which round would ',
    }).state;
    state = applyRealtimeEvent(state, {
      type: 'response.output_audio_transcript.delta',
      delta: 'you like to rehearse?',
    }).state;

    const done = applyRealtimeEvent(state, { type: 'response.done' });
    expect(done.completedTurn).toEqual({
      role: 'assistant',
      content: 'Which round would you like to rehearse?',
    });
    // The user's still-in-progress transcript is untouched by the agent's turn ending.
    expect(done.state.userText).toBe('still arriving');
    expect(done.state.agentText).toBe('');
  });

  it('does not emit an empty turn — a completion with nothing accumulated is not a turn', () => {
    const done = applyRealtimeEvent(initVoiceCallState(), {
      type: 'conversation.item.input_audio_transcription.completed',
    });
    expect(done.completedTurn).toBeUndefined();
  });

  it('does not emit an empty agent turn either', () => {
    const done = applyRealtimeEvent(initVoiceCallState(), { type: 'response.done' });
    expect(done.completedTurn).toBeUndefined();
  });

  it('passes unrelated events through unchanged', () => {
    const state = initVoiceCallState();
    const result = applyRealtimeEvent(state, { type: 'session.created' });
    expect(result.state).toBe(state);
    expect(result.completedTurn).toBeUndefined();
  });
});
