import type { Meta, StoryObj } from '@storybook/svelte';
import Avatar from './avatar.svelte';

const meta = {
  title: 'Primitives/Avatar',
  component: Avatar,
  tags: ['autodocs'],
  argTypes: {
    size: { control: 'select', options: ['sm', 'md', 'lg'] },
  },
} satisfies Meta<Avatar>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { args: { name: 'Jane Doe', size: 'md' } };
export const Small: Story = { args: { name: 'John Smith', size: 'sm' } };
export const Large: Story = { args: { name: 'Alice Wonderland', size: 'lg' } };
export const NoName: Story = { args: { size: 'md' } };
