// Generates the committed PWA manifest icons under web/static/ from the same
// brand mark vector used by the OG cards (web/src/lib/server/og/shared.ts).
// Pure vector render via resvg-js — no satori/font pipeline needed, unlike
// gen-og.mjs, because these icons carry no text.
//
//   node scripts/gen-pwa-icons.mjs   # writes web/static/pwa-*.png; exits non-zero on failure
//
// Re-run and re-commit only if the brand mark itself changes.

import { writeFile } from 'node:fs/promises';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { Resvg } from '@resvg/resvg-js';

const here = dirname(fileURLToPath(import.meta.url));
const staticDir = resolve(here, '../static');

const MARK_PATH =
  'M256 56C366.457 56 456 145.543 456 256C456 366.457 366.457 456 256 456C145.543 456 56 366.457 56 256C56 145.543 145.543 56 256 56ZM256 166L346 256L256 346L166 256L256 166Z';

const PNG_SIGNATURE = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);

function isValidPng(png) {
  return png.subarray(0, 8).equals(PNG_SIGNATURE) && png.length > 200;
}

/** The mark alone, transparent background, filling the 0..512 box as-is
 *  (the path already carries its own ~11% margin — see favicon.svg). */
function markOnlySvg() {
  return (
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512">` +
    `<path fill-rule="evenodd" clip-rule="evenodd" d="${MARK_PATH}" fill="#0a0a0a"/>` +
    `</svg>`
  );
}

/** The mark on an opaque white square, scaled into the centered ~60% safe
 *  zone so Android's adaptive-icon crop (circle, squircle, rounded square,
 *  ...) never clips it. Maskable icons must have no transparency. */
function maskableSvg() {
  const scale = 0.6;
  const offset = (512 * (1 - scale)) / 2;
  return (
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512">` +
    `<rect width="512" height="512" fill="#ffffff"/>` +
    `<g transform="translate(${offset} ${offset}) scale(${scale})">` +
    `<path fill-rule="evenodd" clip-rule="evenodd" d="${MARK_PATH}" fill="#0a0a0a"/>` +
    `</g>` +
    `</svg>`
  );
}

function render(svg, size) {
  return new Resvg(svg, { fitTo: { mode: 'width', value: size } }).render().asPng();
}

async function writeIcon(name, svg, size) {
  const png = Buffer.from(render(svg, size));
  if (!isValidPng(png)) {
    console.error(`Invalid PNG for ${name} (${png.length} bytes)`);
    process.exit(1);
  }
  const outPath = resolve(staticDir, name);
  await writeFile(outPath, png);
  console.log(`Wrote ${outPath} (${png.length} bytes)`);
}

async function main() {
  const mark = markOnlySvg();
  const maskable = maskableSvg();
  await writeIcon('pwa-192x192.png', mark, 192);
  await writeIcon('pwa-512x512.png', mark, 512);
  await writeIcon('pwa-maskable-512x512.png', maskable, 512);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
