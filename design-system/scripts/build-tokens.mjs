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

// Light sources are the base tokens. Dark sources overlay color overrides
// on top of the light ones. Two SD instances — one per theme — because
// platform-level `source` doesn't work (SD only reads root-level `source`).
const lightSources = [
  './tokens/color.tokens.json',
  './tokens/spacing.tokens.json',
  './tokens/typography.tokens.json',
  './tokens/radius.tokens.json',
  './tokens/shadow.tokens.json',
  './tokens/motion.tokens.json',
  './tokens/z-index.tokens.json',
];

const darkSources = [
  ...lightSources,
  './tokens/color-dark.tokens.json',
];

async function buildTheme(sources, destination, selector) {
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
