import type { Meta, StoryObj } from '@storybook/svelte';
import Chip from './chip.svelte';

const meta = {
  title: 'Primitives/Chip',
  component: Chip,
  tags: ['autodocs'],
  argTypes: {
    variant: { control: 'select', options: ['default', 'primary', 'secondary', 'brand', 'destructive'] },
  },
} satisfies Meta<Chip>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { args: { variant: 'default', children: 'Chip' } };
export const Primary: Story = { args: { variant: 'primary', children: 'Active' } };
export const Brand: Story = { args: { variant: 'brand', children: 'Verified' } };
export const Destructive: Story = { args: { variant: 'destructive', children: 'Rejected' } };
