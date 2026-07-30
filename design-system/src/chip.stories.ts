import type { Meta, StoryObj } from '@storybook/svelte';
import { text } from './story-text.js';
import Chip from './chip.svelte';

const meta = {
  title: 'Primitives/Chip',
  component: Chip,
  tags: ['autodocs'],
  argTypes: {
    variant: { control: 'select', options: ['default', 'primary', 'secondary', 'brand', 'destructive'] },
  },
} satisfies Meta<typeof Chip>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { args: { variant: 'default', children: text('Chip') } };
export const Primary: Story = { args: { variant: 'primary', children: text('Active') } };
export const Secondary: Story = { args: { variant: 'secondary', children: text('Remote') } };
export const Brand: Story = { args: { variant: 'brand', children: text('Verified') } };
export const Destructive: Story = { args: { variant: 'destructive', children: text('Rejected') } };
