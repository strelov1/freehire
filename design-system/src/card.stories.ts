import type { Meta, StoryObj } from '@storybook/svelte';
import Card from './card.svelte';

const meta = {
  title: 'Primitives/Card',
  component: Card,
  tags: ['autodocs'],
} satisfies Meta<Card>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { args: { children: 'Card content goes here.' } };
