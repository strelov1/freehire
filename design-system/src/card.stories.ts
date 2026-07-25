import type { Meta, StoryObj } from '@storybook/svelte';
import { text } from './story-text.js';
import Card from './card.svelte';

const meta = {
  title: 'Primitives/Card',
  component: Card,
  tags: ['autodocs'],
} satisfies Meta<typeof Card>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { args: { children: text('Card content goes here.') } };
