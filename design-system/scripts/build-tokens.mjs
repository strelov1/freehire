import { readFileSync } from 'node:fs';
import StyleDictionary from 'style-dictionary';

// Register a custom transform to format shadow objects as CSS box-shadow syntax.
// The built-in shadow/css/shorthand transform expects the SD v3 token structure;
// our DTCG tokens use {x,y,blur,spread,color} objects.
StyleDictionary.registerTransform({
  name: 'shadow/css',
  type: 'value',
  filter: (token) => token.$type === 'shadow' || token.type === 'shadow',
  transform: (token) => {
    const v = token.$value ?? token.value;
    if (typeof v === 'string') return v;
    if (typeof v === 'object' && !Array.isArray(v)) {
      return `${v.x}px ${v.y}px ${v.blur}px ${v.spread}px ${v.color}`;
    }
    return v;
  },
});

StyleDictionary.registerTransformGroup({
  name: 'css-with-shadow',
  transforms: [
    'attribute/cti',
    'name/kebab',
    'time/seconds',
    'size/rem',
    'color/css',
    'fontFamily/css',
    'cubicBezier/css',
    'shadow/css',
  ],
});

// Two SD instances — one per theme — because platform-level `source` doesn't
// work (SD only reads root-level `source`).
//
// Light carries every family; it lands on `:root` and is the whole scale.
const lightSources = [
  './tokens/color.tokens.json',
  './tokens/spacing.tokens.json',
  './tokens/typography.tokens.json',
  './tokens/radius.tokens.json',
  './tokens/shadow.tokens.json',
  './tokens/motion.tokens.json',
  './tokens/z-index.tokens.json',
];

// Dark carries only what dark changes. Overlaying it on the light sources
// would re-declare all 51 non-colour tokens inside `.dark` at their `:root`
// values — a copy the cascade already makes for free, and 24 collision
// warnings that make a real one easy to miss.
const darkSources = ['./tokens/color-dark.tokens.json'];

// Style Dictionary reports a collision as a count with the details behind a
// verbosity flag, so a real one — the same token name authored in two families,
// where the later file silently wins — reads as the number going up by one and
// nothing else. The remaining warnings are all the inert kind: each file carries
// a root `$description` and `$type`, those are the root group's own metadata,
// and merging several files makes the last one win. Nothing consumes root
// metadata, so that collision has no effect; this catches the kind that does.
function assertNoTokenCollision(sources) {
  const owner = new Map();
  const walk = (node, path, file) => {
    for (const [key, value] of Object.entries(node)) {
      if (key.startsWith('$') || typeof value !== 'object' || value === null) continue;
      const here = [...path, key];
      if (!('$value' in value)) {
        walk(value, here, file);
        continue;
      }
      const name = here.join('.');
      const first = owner.get(name);
      if (first) {
        throw new Error(
          `token "${name}" is authored in both ${first} and ${file} — ` +
            `the later file wins silently. Rename one, or make the second an ` +
            `override built into its own theme.`,
        );
      }
      owner.set(name, file);
    }
  };
  for (const file of sources) walk(JSON.parse(readFileSync(file, 'utf-8')), [], file);
}

async function buildTheme(sources, destination, selector) {
  assertNoTokenCollision(sources);
  const sd = new StyleDictionary({
    source: sources,
    platforms: {
      theme: {
        transformGroup: 'css-with-shadow',
        buildPath: 'dist/',
        files: [
          {
            destination,
            format: 'css/variables',
            options: { selector, showFileHeader: false },
          },
        ],
      },
    },
  });
  await sd.buildAllPlatforms();
}

await buildTheme(lightSources, 'tokens-light.css', ':root');
await buildTheme(darkSources, 'tokens-dark.css', '.dark');
