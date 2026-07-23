import type { Meta, StoryObj } from '@storybook/svelte';
import Input from './input.svelte';

const meta = {
  title: 'Primitives/Input',
  component: Input,
  tags: ['autodocs'],
} satisfies Meta<Input>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { args: { placeholder: 'Enter text...' } };
export const WithValue: Story = { args: { value: 'hello@freehire.dev' } };
export const Disabled: Story = { args: { placeholder: 'Disabled', disabled: true } };
