import type { Meta, StoryObj } from '@storybook/svelte';
import SettingRowDemo from '../.storybook/demos/SettingRowDemo.svelte';

const meta = {
  title: 'Primitives/SettingRow',
  component: SettingRowDemo,
  tags: ['autodocs'],
} satisfies Meta<typeof SettingRowDemo>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { args: { label: 'Font' } };
export const WithHint: Story = { args: { label: 'Font', hint: 'Used across the whole document.' } };
export const Grow: Story = { args: { label: 'Font', grow: true } };
