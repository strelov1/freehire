import type { Meta, StoryObj } from '@storybook/svelte';
import LoadMore from './load-more.svelte';

const meta = {
  title: 'Primitives/LoadMore',
  component: LoadMore,
  tags: ['autodocs'],
  args: { onclick: () => {} },
} satisfies Meta<typeof LoadMore>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Idle: Story = { args: { loading: false } };
export const Loading: Story = { args: { loading: true } };
export const Error: Story = { args: { loading: false, error: true } };
