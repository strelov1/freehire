import { BASE_URL, type Group } from '../api-spec-types';

export const photoGroup: Group = {
  title: 'Photo',
  intro:
    'The one image the CV templates that print a portrait compose in. ' +
    'Cookie-only throughout — the image is PII, so there is no key-authenticated ' +
    'or public path.',
  endpoints: [
    {
      method: 'PUT',
      path: '/me/photo',
      auth: 'cookie',
      summary: 'Upload (or replace) your headshot.',
      description:
        'Multipart upload of the `file` part — JPEG, PNG, or WebP, capped at 8 MB ' +
        'and replaces any existing headshot. ' +
        '`501` when object storage is not configured, `400` for an undecodable, ' +
        'unsupported, or oversized image.',
      curl: `curl -X PUT "${BASE_URL}/me/photo" -b cookies.txt -F "file=@headshot.jpg"`,
      responseExample: `{ "data": { "enabled": true, "present": true, "uploaded_at": "2026-08-18T10:00:00Z" } }`,
    },
    {
      method: 'GET',
      path: '/me/photo',
      auth: 'cookie',
      summary: 'Whether you have a stored headshot, and when it was uploaded.',
      description:
        '`enabled` is false when object storage is not configured — the client ' +
        'should then offer no upload control at all rather than one that answers ' +
        '`501`. `uploaded_at` doubles as a cache-busting value for the image URL.',
      curl: `curl "${BASE_URL}/me/photo" -b cookies.txt`,
      responseExample: `{ "data": { "enabled": true, "present": false, "uploaded_at": null } }`,
    },
    {
      method: 'GET',
      path: '/me/photo/image',
      auth: 'cookie',
      summary: 'The stored headshot itself, as image bytes.',
      description:
        'Not a JSON endpoint — the response is `image/jpeg` with a short private ' +
        'cache lifetime (60s), meant to be busted with `?v=<uploaded_at>`. `404` ' +
        'when no headshot is stored.',
      curl: `curl "${BASE_URL}/me/photo/image" -b cookies.txt -o headshot.jpg`,
    },
    {
      method: 'DELETE',
      path: '/me/photo',
      auth: 'cookie',
      summary: 'Remove your stored headshot.',
      description: 'Returns `204 No Content`.',
      curl: `curl -X DELETE "${BASE_URL}/me/photo" -b cookies.txt`,
    },
  ],
};
