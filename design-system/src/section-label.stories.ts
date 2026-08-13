import type { Meta, StoryObj } from '@storybook/svelte';
import SectionLabel from './section-label.svelte';

const meta = {
  title: 'Primitives/SectionLabel',
  component: SectionLabel,
  tags: ['autodocs'],
} satisfies Meta<typeof SectionLabel>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { args: { text: 'How it works' } };
