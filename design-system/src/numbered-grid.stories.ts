import type { Meta, StoryObj } from '@storybook/svelte';
import NumberedGrid from './numbered-grid.svelte';

const meta = {
  title: 'Primitives/NumberedGrid',
  component: NumberedGrid,
  tags: ['autodocs'],
} satisfies Meta<typeof NumberedGrid>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    items: [
      { n: '01', title: 'Aggregate', body: 'Every board in one place, deduplicated and normalized.' },
      { n: '02', title: 'Enrich', body: 'AI fills in the gaps a raw posting leaves out.' },
      { n: '03', title: 'Apply', body: 'Tailor a CV to the role and track it through to an offer.' },
    ],
  },
};
