// The app's single door onto the design system: app code imports from `$lib/ui`
// and never names the package. Re-exported wholesale rather than primitive by
// primitive — an enumerated list is a second copy of the package's export surface,
// and it drifted once already, leaving eleven of the fifteen primitives built,
// tested, storybooked and unreachable from here with every check still green.
export * from 'freehire-design-system';
