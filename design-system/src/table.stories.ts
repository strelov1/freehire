import type { Meta, StoryObj } from '@storybook/svelte';
import TableDemo from '../.storybook/demos/TableDemo.svelte';

const meta = {
  title: 'Primitives/Table',
  component: TableDemo,
  tags: ['autodocs'],
} satisfies Meta<typeof TableDemo>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
// tbody's `[&_tr:last-child]:border-0` is only visible with more than one row,
// and the hover tint is what a single row cannot show at all.
export const SingleRow: Story = {
  args: { rows: [{ role: 'Senior Go Engineer', company: 'Granola', status: 'Applied' }] },
};
