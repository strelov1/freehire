import type { Meta, StoryObj } from '@storybook/svelte';
import Button from './button.svelte';

const meta = {
  title: 'Primitives/Button',
  component: Button,
  tags: ['autodocs'],
  argTypes: {
    variant: { control: 'select', options: ['primary', 'secondary', 'outline', 'ghost'] },
    size: { control: 'select', options: ['sm', 'md', 'lg', 'icon'] },
  },
} satisfies Meta<Button>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { args: { variant: 'secondary', size: 'md', children: 'Click me' } };
export const Primary: Story = { args: { variant: 'primary', size: 'md', children: 'Primary' } };
export const Outline: Story = { args: { variant: 'outline', size: 'md', children: 'Outline' } };
export const Ghost: Story = { args: { variant: 'ghost', size: 'md', children: 'Ghost' } };
export const Small: Story = { args: { variant: 'secondary', size: 'sm', children: 'Small' } };
export const Large: Story = { args: { variant: 'primary', size: 'lg', children: 'Large' } };
