// Loads the bundled Inter weights for satori. `read()` from $app/server is the
// adapter-agnostic way to read a Vite-imported asset at runtime, so this works the
// same in dev and in the built Node server. The fonts never change within a process,
// so the result is memoised — the file read happens once.

import { read } from '$app/server';
import type { OgFont } from './render';
import InterRegular from './fonts/Inter-Regular.ttf';
import InterSemiBold from './fonts/Inter-SemiBold.ttf';
import InterBold from './fonts/Inter-Bold.ttf';

let cache: OgFont[] | null = null;

export async function loadOgFonts(): Promise<OgFont[]> {
  if (cache) return cache;
  const [regular, semibold, bold] = await Promise.all([
    read(InterRegular).arrayBuffer(),
    read(InterSemiBold).arrayBuffer(),
    read(InterBold).arrayBuffer(),
  ]);
  cache = [
    { name: 'Inter', data: regular, weight: 400, style: 'normal' },
    { name: 'Inter', data: semibold, weight: 600, style: 'normal' },
    { name: 'Inter', data: bold, weight: 700, style: 'normal' },
  ];
  return cache;
}
