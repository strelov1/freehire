import type { Meta, StoryObj } from '@storybook/svelte';
import EmptyState from './empty-state.svelte';

const meta = {
  title: 'Primitives/EmptyState',
  component: EmptyState,
  tags: ['autodocs'],
} satisfies Meta<EmptyState>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: { title: 'No results found', description: 'Try adjusting your filters to see more jobs.' },
};
export const Minimal: Story = { args: { title: 'Nothing here yet' } };
