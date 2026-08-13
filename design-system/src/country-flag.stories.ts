import type { Meta, StoryObj } from '@storybook/svelte';
// Story-local, per design.md: the package does not carry flag-icons as a runtime
// dependency of the primitive itself, only as this story's own asset — the same
// pattern button-icon.stories.ts uses for a story-only dependency.
import 'flag-icons/css/flag-icons.min.css';
import CountryFlag from './country-flag.svelte';

const meta = {
  title: 'Primitives/CountryFlag',
  component: CountryFlag,
  tags: ['autodocs'],
} satisfies Meta<typeof CountryFlag>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Renderable: Story = { args: { code: 'de', label: 'Germany' } };

// Japan and Nigeria are the two flags this primitive's ring exists for — both carry
// a near-white edge that disappears against a light background without it.
export const LightFlagOnLightBackground: Story = { args: { code: 'jp', label: 'Japan' } };

export const Unrenderable: Story = { args: { code: 'remote', label: 'Remote' } };
