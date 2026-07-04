// Framework-free render core for the OG images: HTML markup + fonts → PNG.
//
// It deliberately imports no SvelteKit runtime ($app/*), so it runs both inside the
// SSR server (via the og.png endpoints, which supply fonts/logo) and under a plain
// Node smoke test. Font loading and logo fetching live in their own modules, and
// the card builders (job/company/brand) live beside this, so this stays a pure
// transform of its inputs.

import satori from 'satori';
import { html } from 'satori-html';
import { Resvg } from '@resvg/resvg-js';
import type { Job } from '$lib/generated/contracts';
import { buildCard } from './card';
import { OG_HEIGHT, OG_WIDTH } from './shared';

export type OgFont = {
  name: string;
  data: Buffer | ArrayBuffer;
  weight?: 400 | 600 | 700;
  style?: 'normal';
};

/** Renders a card's HTML markup string to a 1200×630 PNG. The single render path
 *  shared by every card (job, company, brand). */
export async function renderMarkupPng(markup: string, fonts: OgFont[]): Promise<Uint8Array<ArrayBuffer>> {
  const svg = await satori(html(markup), { width: OG_WIDTH, height: OG_HEIGHT, fonts });
  const png = new Resvg(svg, { fitTo: { mode: 'width', value: OG_WIDTH } }).render().asPng();
  // resvg returns a Node Buffer (Uint8Array<ArrayBufferLike>); copy into a plain
  // ArrayBuffer-backed view so it satisfies the web BodyInit type at the endpoint.
  const out = new Uint8Array(png.byteLength);
  out.set(png);
  return out;
}

/** Renders the job's OG card to a PNG. `logo` is a data-URI or null (monogram). */
export async function renderCardPng(
  job: Job,
  opts: { fonts: OgFont[]; logo: string | null },
): Promise<Uint8Array<ArrayBuffer>> {
  return renderMarkupPng(buildCard(job, { logo: opts.logo }), opts.fonts);
}
