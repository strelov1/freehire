import type { Meta, StoryObj } from '@storybook/svelte';
import Pagination from './pagination.svelte';

const meta = {
  title: 'Primitives/Pagination',
  component: Pagination,
  tags: ['autodocs'],
} satisfies Meta<Pagination>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { args: { page: 1, total: 250, perPage: 20 } };
export const LastPage: Story = { args: { page: 13, total: 250, perPage: 20 } };
export const SinglePage: Story = { args: { page: 1, total: 5, perPage: 20 } };
