import { BASE_URL, type Group } from '../api-spec-types';

export const speechGroup: Group = {
  title: 'Speech',
  intro:
    'Turns one recording into text for the assistant composer’s dictation ' +
    'control. Nothing is stored — the audio is read, forwarded to the ' +
    'transcription gateway, and dropped.',
  endpoints: [
    {
      method: 'POST',
      path: '/speech/transcriptions',
      auth: 'cookie-or-key',
      summary: 'Transcribe one short audio recording to text.',
      description:
        'Multipart upload of the `file` part — webm, mp4, m4a, ogg, wav, or mp3, ' +
        'capped at ~8 minutes of audio (2 MB). `501` when no speech gateway is configured, ' +
        '`502` if the gateway call itself fails, `400` for an empty recording.',
      curl: `curl -X POST "${BASE_URL}/speech/transcriptions" \\
  -H "Authorization: Bearer $FREEHIRE_API_KEY" \\
  -F "file=@dictation.webm"`,
      responseExample: `{ "data": { "text": "remote go engineer roles in Berlin" } }`,
    },
  ],
};
