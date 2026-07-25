import type { Meta, StoryObj } from '@storybook/svelte';
import { text } from './story-text.js';
import Badge from './badge.svelte';

const meta = {
  title: 'Primitives/Badge',
  component: Badge,
  tags: ['autodocs'],
  argTypes: {
    variant: { control: 'select', options: ['secondary', 'outline', 'brand', 'missing'] },
  },
} satisfies Meta<typeof Badge>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Secondary: Story = { args: { variant: 'secondary', children: text('Badge') } };
export const Outline: Story = { args: { variant: 'outline', children: text('Badge') } };
export const Brand: Story = { args: { variant: 'brand', children: text('New') } };
export const Missing: Story = { args: { variant: 'missing', children: text('Missing') } };
