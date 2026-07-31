// The parts of dictation that are decisions rather than device handling: what the
// draft becomes, whether this browser can record at all, and which container to ask
// for. VoiceInput.svelte owns the stream, the recorder and the clock; this owns the
// rules, so they can be tested without a microphone.

/** How long one recording may run before it stops itself.
 *
 *  Two reasons for a ceiling, and the tighter one wins. Transcription is billed per
 *  minute of audio against an assistant that is not metered, so a microphone left open
 *  is unbounded spend. And the server refuses an upload past 2 MiB — at the ~32 kbit/s
 *  the browser's opus encoder produces, five minutes is about 1.2 MB, which leaves room
 *  rather than discovering the limit after someone has finished speaking. */
export const MAX_RECORDING_MS = 5 * 60 * 1000;

/** The bits of `window` recording needs, as a shape rather than as the global, so the
 *  support check can be exercised for browsers this test run is not. */
export type RecorderEnv = {
  mediaDevices?: { getUserMedia?: unknown };
  MediaRecorder?: unknown;
};

/** Whether this browser can record.
 *
 *  Both halves are load-bearing. `MediaRecorder` is simply absent in some browsers;
 *  `mediaDevices` is absent outside a secure context, which is what an insecure origin
 *  looks like from here. The composer renders no microphone when this is false —
 *  a control that can only fail teaches people to stop reaching for it. */
export function canRecord(env: RecorderEnv): boolean {
  return Boolean(env.MediaRecorder && env.mediaDevices?.getUserMedia);
}

/** A container this browser can produce and our gateway can read. */
export type Container = { mimeType: string; filename: string };

/** Containers in the order we would rather have them: opus first for size, then
 *  whatever the browser will give us. Safari supports none of the webm variants and
 *  produces mp4; some Linux builds produce ogg.
 *
 *  The filename matters as much as the type. The gateway identifies the container by
 *  extension, and the server rebuilds the name from an allowlist of exactly these — so
 *  a container added here without its server-side counterpart is a 400 the caller
 *  cannot explain. */
const CONTAINERS: Container[] = [
  { mimeType: 'audio/webm;codecs=opus', filename: 'dictation.webm' },
  { mimeType: 'audio/webm', filename: 'dictation.webm' },
  { mimeType: 'audio/ogg;codecs=opus', filename: 'dictation.ogg' },
  { mimeType: 'audio/mp4', filename: 'dictation.mp4' },
  { mimeType: 'audio/wav', filename: 'dictation.wav' },
];

/** The first container this browser supports, or null if it supports none of them.
 *  Takes `MediaRecorder.isTypeSupported` as an argument rather than reading the global,
 *  so the fallback order is testable. */
export function pickContainer(isTypeSupported: (type: string) => boolean): Container | null {
  return CONTAINERS.find((c) => isTypeSupported(c.mimeType)) ?? null;
}

/** What the draft becomes once a recording has been transcribed.
 *
 *  The transcription is appended, never substituted: whatever was typed is the
 *  caller's and survives. Silence transcribes to an empty string, and appending
 *  nothing for it is the right answer — a stray space would be a change they cannot
 *  see and did not ask for. */
export function appendTranscript(draft: string, text: string): string {
  const said = text.trim();
  if (!said) return draft;
  if (!draft) return said;
  return /\s$/.test(draft) ? draft + said : `${draft} ${said}`;
}
