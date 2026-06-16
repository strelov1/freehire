// Framework-free render core for job OG images: job + fonts + resolved logo → PNG.
//
// It deliberately imports no SvelteKit runtime ($app/*), so it runs both inside the
// SSR server (via the og.png endpoint, which supplies fonts/logo) and under a plain
// Node smoke test. Font loading and logo fetching live in their own modules so this
// stays a pure transform of its inputs.

import satori from 'satori';
import { html } from 'satori-html';
import { Resvg } from '@resvg/resvg-js';
import type { Job } from '$lib/generated/contracts';
import { OG_HEIGHT, OG_WIDTH, buildCard } from './card';

export type OgFont = {
  name: string;
  data: Buffer | ArrayBuffer;
  weight?: 400 | 600 | 700;
  style?: 'normal';
};

/** Renders the job's OG card to a PNG. `logo` is a data-URI or null (monogram). */
export async function renderCardPng(
  job: Job,
  opts: { fonts: OgFont[]; logo: string | null },
): Promise<Uint8Array<ArrayBuffer>> {
  const markup = html(buildCard(job, { logo: opts.logo }));
  const svg = await satori(markup, { width: OG_WIDTH, height: OG_HEIGHT, fonts: opts.fonts });
  const png = new Resvg(svg, { fitTo: { mode: 'width', value: OG_WIDTH } }).render().asPng();
  // resvg returns a Node Buffer (Uint8Array<ArrayBufferLike>); copy into a plain
  // ArrayBuffer-backed view so it satisfies the web BodyInit type at the endpoint.
  const out = new Uint8Array(png.byteLength);
  out.set(png);
  return out;
}
