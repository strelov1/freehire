import type { Meta, StoryObj } from '@storybook/svelte';
import Alert from './alert.svelte';

const meta = {
  title: 'Primitives/Alert',
  component: Alert,
  tags: ['autodocs'],
  argTypes: {
    variant: { control: 'select', options: ['default', 'destructive', 'brand'] },
  },
} satisfies Meta<Alert>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { args: { variant: 'default', children: 'This is an informational alert.' } };
export const Destructive: Story = { args: { variant: 'destructive', children: 'Something went wrong.' } };
export const Brand: Story = { args: { variant: 'brand', children: 'Profile is complete!' } };
