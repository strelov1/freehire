import { describe, it, expect } from 'vitest';
import { appendTranscript, canRecord, pickContainer, MAX_RECORDING_MS } from './dictation';

describe('appendTranscript', () => {
  it('becomes the draft when there was nothing typed', () => {
    expect(appendTranscript('', 'find me remote go jobs')).toBe('find me remote go jobs');
  });

  it('is separated from typed text by a single space', () => {
    expect(appendTranscript('also', 'in Berlin')).toBe('also in Berlin');
  });

  it('does not add a second space when the draft already ends in one', () => {
    expect(appendTranscript('also ', 'in Berlin')).toBe('also in Berlin');
    expect(appendTranscript('also\n', 'in Berlin')).toBe('also\nin Berlin');
  });

  it('leaves the draft alone when nothing was said', () => {
    // Whisper answers an empty string for audio with no speech in it. Appending a
    // stray space for a recording of silence would be a change the caller cannot see.
    expect(appendTranscript('also', '')).toBe('also');
    expect(appendTranscript('also', '   ')).toBe('also');
    expect(appendTranscript('', '')).toBe('');
  });

  it('trims the transcription without touching what was typed', () => {
    expect(appendTranscript('also', '  in Berlin  ')).toBe('also in Berlin');
    expect(appendTranscript('  ', 'in Berlin')).toBe('  in Berlin');
  });
});

describe('canRecord', () => {
  it('is true only when both APIs are present', () => {
    expect(canRecord({ mediaDevices: { getUserMedia: () => {} }, MediaRecorder: {} })).toBe(
      true,
    );
  });

  it('is false without MediaRecorder', () => {
    expect(canRecord({ mediaDevices: { getUserMedia: () => {} } })).toBe(false);
  });

  it('is false without getUserMedia, which is what an insecure context looks like', () => {
    expect(canRecord({ MediaRecorder: {} })).toBe(false);
    expect(canRecord({ mediaDevices: {}, MediaRecorder: {} })).toBe(false);
  });

  it('is false in a bare environment', () => {
    expect(canRecord({})).toBe(false);
  });
});

describe('pickContainer', () => {
  it('prefers opus in webm, which is what Chrome and Firefox produce', () => {
    const picked = pickContainer(() => true);
    expect(picked?.mimeType).toBe('audio/webm;codecs=opus');
    expect(picked?.filename).toBe('dictation.webm');
  });

  it('falls back to what the browser actually supports', () => {
    // Safari supports none of the webm variants.
    const picked = pickContainer((type) => type.startsWith('audio/mp4'));
    expect(picked?.mimeType).toBe('audio/mp4');
    expect(picked?.filename).toBe('dictation.mp4');
  });

  it('names the file by the container, because the gateway sniffs the extension', () => {
    const picked = pickContainer((type) => type === 'audio/ogg;codecs=opus');
    expect(picked?.filename).toBe('dictation.ogg');
  });

  it('is null when the browser supports nothing we can send', () => {
    expect(pickContainer(() => false)).toBeNull();
  });
});

describe('MAX_RECORDING_MS', () => {
  it('stays under the size the server will accept', () => {
    // The server caps an upload at 2 MiB. The browser's opus encoder runs at roughly
    // 32 kbit/s, so the ceiling has to leave room under that or a caller would be told
    // their recording was refused only after they had finished speaking.
    const bytesPerSecond = 32_000 / 8;
    expect((MAX_RECORDING_MS / 1000) * bytesPerSecond).toBeLessThan(2 * 1024 * 1024);
  });
});
