import type { Meta, StoryObj } from '@storybook/svelte';
import Skeleton from './skeleton.svelte';

const meta = {
  title: 'Primitives/Skeleton',
  component: Skeleton,
  tags: ['autodocs'],
} satisfies Meta<Skeleton>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { args: { class: 'h-4 w-48' } };
export const Circle: Story = { args: { class: 'size-12 rounded-full' } };
export const Card: Story = { args: { class: 'h-32 w-full' } };
