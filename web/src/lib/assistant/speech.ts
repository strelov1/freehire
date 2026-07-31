// The transcription call. It sits outside api.ts because it is not an assistant
// endpoint and not JSON: /api/v1/speech/transcriptions takes one multipart upload,
// and setting a content type by hand would strip the boundary the browser generates.

/** Thrown when the deployment has no speech gateway (501). Its own type because the
 *  composer's answer is to REMOVE the microphone rather than to report a failure:
 *  the feature is absent here, not broken. */
export class TranscriptionUnavailable extends Error {
  constructor() {
    super('transcription is not configured');
    this.name = 'TranscriptionUnavailable';
  }
}

/** The words in a recording.
 *
 *  An empty string is a normal answer — it is what silence transcribes to — so callers
 *  must treat it as "nothing was said" rather than as a failure. */
export async function transcribe(audio: Blob, filename: string): Promise<string> {
  const form = new FormData();
  form.append('file', audio, filename);

  const res = await fetch('/api/v1/speech/transcriptions', {
    method: 'POST',
    credentials: 'include',
    body: form,
  });
  if (res.status === 501) throw new TranscriptionUnavailable();
  if (res.status === 429) throw new Error('Too many recordings just now — try again shortly.');
  if (!res.ok) throw new Error(`Could not transcribe the recording (${res.status}).`);

  const body = (await res.json()) as { data?: { text?: string } };
  return body.data?.text ?? '';
}
