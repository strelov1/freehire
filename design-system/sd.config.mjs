// Style Dictionary config — DTCG-native, two-platform theming.
//
// Light values are the base $value in each .tokens.json; dark overrides live
// in color-dark.tokens.json. Two platforms emit two CSS blocks (:root for
// light, .dark for dark). Aliases ({brand-ring}) resolve per-platform so ring
// follows the correct brand-ring value in each theme.

const lightSources = [
  './tokens/color.tokens.json',
  './tokens/spacing.tokens.json',
  './tokens/typography.tokens.json',
  './tokens/radius.tokens.json',
  './tokens/shadow.tokens.json',
  './tokens/motion.tokens.json',
  './tokens/z-index.tokens.json',
];

export default {
  log: 'warn',
  platforms: {
    light: {
      source: lightSources,
      transformGroup: 'css',
      buildPath: 'dist/',
      files: [
        {
          destination: 'tokens-light.css',
          format: 'css/variables',
          options: {
            selector: ':root',
            showFileHeader: false,
          },
        },
      ],
    },
    dark: {
      // Include light sources so aliases (e.g. {brand-ring}) resolve,
      // then overlay dark $values from color-dark.tokens.json.
      include: lightSources,
      source: ['./tokens/color-dark.tokens.json'],
      transformGroup: 'css',
      buildPath: 'dist/',
      files: [
        {
          destination: 'tokens-dark.css',
          format: 'css/variables',
          options: {
            selector: '.dark',
            showFileHeader: false,
          },
        },
      ],
    },
  },
};
