// Which glyph fill to draw over a brand's own background color, so a logo or
// family icon stays legible regardless of how light or dark that brand's hex is.
const LUMINANCE_THRESHOLD = 128;

export function glyphColorFor(hex: string): string {
  const value = hex.replace('#', '');
  const r = parseInt(value.slice(0, 2), 16);
  const g = parseInt(value.slice(2, 4), 16);
  const b = parseInt(value.slice(4, 6), 16);
  const yiq = (r * 299 + g * 587 + b * 114) / 1000;
  return yiq >= LUMINANCE_THRESHOLD ? '#16181d' : '#ffffff';
}
