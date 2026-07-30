import type { Meta, StoryObj } from '@storybook/svelte';
import FormFieldDemo from '../.storybook/demos/FormFieldDemo.svelte';

const meta = {
  title: 'Primitives/FormField',
  component: FormFieldDemo,
  tags: ['autodocs'],
} satisfies Meta<typeof FormFieldDemo>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { args: { label: 'Work email' } };
export const WithHint: Story = {
  args: { label: 'Work email', hint: 'Only used to verify the employer domain.' },
};
export const Required: Story = { args: { label: 'Work email', required: true } };
// `error` wins over `hint`: one message element, and the field points
// aria-describedby at whichever is showing.
export const WithError: Story = {
  args: {
    label: 'Work email',
    hint: 'Only used to verify the employer domain.',
    error: 'That address is already on another account.',
    required: true,
  },
};
