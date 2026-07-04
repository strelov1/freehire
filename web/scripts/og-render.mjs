// Shared bootstrap for the dependency-free OG render scripts (og-smoke.mjs and
// gen-og.mjs). Both load the TypeScript render core through Vite's SSR loader
// (Vite is already a devDependency) so the source stays in the project's normal
// extensionless / $lib style — no .ts-extension imports, no tsconfig changes, no
// new framework — and both read the same bundled Inter weights and assert the same
// PNG validity. Kept here so the font set and loader config live in one place.
//
// The app's own font loader (src/lib/server/og/fonts.ts) can't be reused: it uses
// `read()` from $app/server, which only exists under the SvelteKit runtime these
// bare Vite scripts don't have.

import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';
import { createServer } from 'vite';

const here = dirname(fileURLToPath(import.meta.url));
export const webRoot = resolve(here, '..');
const fontsDir = resolve(webRoot, 'src/lib/server/og/fonts');

const PNG_SIGNATURE = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);

/** A satori-shaped valid PNG: correct signature and non-trivially sized. */
export function isValidPng(png) {
  return png.subarray(0, 8).equals(PNG_SIGNATURE) && png.length > 2000;
}

/** A Vite dev server configured to SSR-load the web/src TypeScript OG modules. */
export function createOgVite() {
  return createServer({
    configFile: false,
    root: webRoot,
    logLevel: 'error',
    resolve: { alias: { $lib: resolve(webRoot, 'src/lib') } },
    ssr: { external: ['@resvg/resvg-js'] },
  });
}

/** The bundled Inter weights in satori's font shape. */
export function loadFonts() {
  const files = [
    ['Inter-Regular.ttf', 400],
    ['Inter-SemiBold.ttf', 600],
    ['Inter-Bold.ttf', 700],
  ];
  return Promise.all(
    files.map(async ([file, weight]) => ({
      name: 'Inter',
      data: await readFile(resolve(fontsDir, file)),
      weight,
      style: 'normal',
    })),
  );
}
