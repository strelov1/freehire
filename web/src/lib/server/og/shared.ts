// Shared primitives for the Open Graph cards (job, company, brand). Pure and
// framework-free so every card speaks one brand vocabulary and cannot drift, and
// so the render smoke tests can exercise them directly.
//
// satori constraint: layout is flexbox only (no CSS grid), and any element with
// more than one child declares `display: flex`.

export const OG_WIDTH = 1200;
export const OG_HEIGHT = 630;

// The freehire brand mark (a circle with a diamond cut-out), inlined as a base64
// data-URI so satori embeds it with no network fetch (satori cannot fetch remote
// images). Coloured #0a0a0a to match the footer wordmark beside it.
const MARK_SVG =
  '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512">' +
  '<path fill-rule="evenodd" clip-rule="evenodd" d="M256 56C366.457 56 456 145.543 456 256C456 366.457 366.457 456 256 456C145.543 456 56 366.457 56 256C56 145.543 145.543 56 256 56ZM256 166L346 256L256 346L166 256L256 166Z" fill="#0a0a0a"/>' +
  '</svg>';
export const MARK_DATA_URI = `data:image/svg+xml;base64,${Buffer.from(MARK_SVG).toString('base64')}`;

export function esc(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

/** One or two uppercase initials from a name, for the logo fallback tile. */
export function monogram(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean);
  const first = words[0];
  if (!first) return '?';
  if (words.length === 1) return first.slice(0, 2).toUpperCase();
  return (first.charAt(0) + (words[1] ?? '').charAt(0)).toUpperCase();
}

/** A square logo tile at `size`px: the resolved logo image, or a monogram tile
 *  derived from `name` when `logo` is null. Radius and monogram font scale with
 *  the size (at 72px this matches the original job-card tile: radius 14, font 30). */
export function logoBox(logo: string | null, name: string, size: number): string {
  const radius = Math.round(size / 5);
  if (logo) {
    // satori reads image dimensions from style, not the width/height HTML attributes.
    return `<img src="${esc(logo)}" style="width:${size}px;height:${size}px;border-radius:${radius}px;object-fit:contain" />`;
  }
  const font = Math.round(size * 0.42);
  return (
    `<div style="display:flex;align-items:center;justify-content:center;width:${size}px;height:${size}px;` +
    `border-radius:${radius}px;background:#f4f4f4;color:#525252;font-size:${font}px;font-weight:700">` +
    `${esc(monogram(name))}</div>`
  );
}

export type Chip = { text: string; muted?: boolean };

export function chipMarkup(chip: Chip): string {
  const style = chip.muted
    ? 'display:flex;align-items:center;color:#a3a3a3;font-size:24px;font-weight:500;padding:8px 4px'
    : 'display:flex;align-items:center;background:#f4f4f4;color:#171717;font-size:24px;font-weight:500;border-radius:999px;padding:9px 18px';
  return `<div style="${style}">${esc(chip.text)}</div>`;
}

/** The shared bottom row: the freehire mark + wordmark on the left, the site
 *  domain on the right. Every card closes with this. */
export function brandFooter(): string {
  return `
  <div style="display:flex;align-items:center;justify-content:space-between">
    <div style="display:flex;align-items:center;gap:14px">
      <img src="${MARK_DATA_URI}" style="width:34px;height:34px" />
      <div style="display:flex;font-size:30px;font-weight:700;letter-spacing:-0.03em">freehire</div>
    </div>
    <div style="display:flex;font-size:22px;color:#a3a3a3">freehire.dev</div>
  </div>`;
}
