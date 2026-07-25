import type { Meta, StoryObj } from '@storybook/svelte';
import { text } from './story-text.js';
import Button from './button.svelte';

const meta = {
  title: 'Primitives/Button',
  component: Button,
  tags: ['autodocs'],
  argTypes: {
    variant: { control: 'select', options: ['primary', 'secondary', 'outline', 'ghost'] },
    size: { control: 'select', options: ['sm', 'md', 'lg', 'icon'] },
  },
} satisfies Meta<typeof Button>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { args: { variant: 'secondary', size: 'md', children: text('Click me') } };
export const Primary: Story = { args: { variant: 'primary', size: 'md', children: text('Primary') } };
export const Outline: Story = { args: { variant: 'outline', size: 'md', children: text('Outline') } };
export const Ghost: Story = { args: { variant: 'ghost', size: 'md', children: text('Ghost') } };
export const Small: Story = { args: { variant: 'secondary', size: 'sm', children: text('Small') } };
export const Large: Story = { args: { variant: 'primary', size: 'lg', children: text('Large') } };
