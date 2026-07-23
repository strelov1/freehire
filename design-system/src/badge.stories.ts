import type { Meta, StoryObj } from '@storybook/svelte';
import Badge from './badge.svelte';

const meta = {
  title: 'Primitives/Badge',
  component: Badge,
  tags: ['autodocs'],
  argTypes: {
    variant: { control: 'select', options: ['secondary', 'outline', 'brand', 'missing'] },
  },
} satisfies Meta<Badge>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Secondary: Story = { args: { variant: 'secondary', children: 'Badge' } };
export const Outline: Story = { args: { variant: 'outline', children: 'Badge' } };
export const Brand: Story = { args: { variant: 'brand', children: 'New' } };
export const Missing: Story = { args: { variant: 'missing', children: 'Missing' } };
