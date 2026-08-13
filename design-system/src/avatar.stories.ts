import type { Meta, StoryObj } from '@storybook/svelte';
import Avatar from './avatar.svelte';

const meta = {
  title: 'Primitives/Avatar',
  component: Avatar,
  tags: ['autodocs'],
  argTypes: {
    size: { control: 'select', options: ['xs', 'sm', 'md', 'lg'] },
    shape: { control: 'select', options: ['circle', 'square'] },
  },
} satisfies Meta<typeof Avatar>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { args: { name: 'Jane Doe', size: 'md' } };
// The tightest tile CompanyLogo's call sites needed — a dense row of inline logos.
export const ExtraSmall: Story = { args: { name: 'Jane Doe', size: 'xs' } };
export const Small: Story = { args: { name: 'John Smith', size: 'sm' } };
export const Large: Story = { args: { name: 'Alice Wonderland', size: 'lg' } };
export const NoName: Story = { args: { size: 'md' } };

// A genuinely broken URL, so the browser's own 404 drives the same onerror/catchMissedError
// path a real dead logo link hits — the story shows the initials render it falls back to.
export const BrokenPhoto: Story = {
  args: { name: 'Ada Lovelace', src: 'https://example.invalid/does-not-exist.png' },
};
