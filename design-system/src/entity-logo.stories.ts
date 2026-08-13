import type { Meta, StoryObj } from '@storybook/svelte';
// EntityLogo is a second name for avatar.svelte (see index.ts) — the company/entity
// tile, as distinct from Avatar's person-circle default. Its own story file exists
// because it is its own export in the census check:adoption reads, and because
// "square shape, no name → globe" is the shape this name exists to show, not a buried
// variant of Avatar's.
import EntityLogo from './avatar.svelte';

const meta = {
  title: 'Primitives/EntityLogo',
  component: EntityLogo,
  tags: ['autodocs'],
  argTypes: {
    size: { control: 'select', options: ['xs', 'sm', 'md', 'lg'] },
  },
  args: { shape: 'square' },
} satisfies Meta<typeof EntityLogo>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { args: { name: 'Acme Corp', size: 'sm' } };
export const ExtraSmall: Story = { args: { name: 'Acme Corp', size: 'xs' } };
export const Large: Story = { args: { name: 'Acme Corp', size: 'lg' } };

// A genuinely broken logo URL — the same onerror/catchMissedError path a dead company
// logo link hits, falling back to the monogram.
export const BrokenPhoto: Story = {
  args: { name: 'Acme Corp', src: 'https://example.invalid/does-not-exist.png', size: 'sm' },
};
