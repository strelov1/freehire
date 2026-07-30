import type { Meta, StoryObj } from '@storybook/svelte';
import { text } from './story-text.js';
import Tabs from './tabs.svelte';

const meta = {
  title: 'Primitives/Tabs',
  component: Tabs,
  tags: ['autodocs'],
} satisfies Meta<typeof Tabs>;

export default meta;
type Story = StoryObj<typeof meta>;

const tabs = [
  { value: 'overview', label: 'Overview' },
  { value: 'skills', label: 'Skills' },
  { value: 'company', label: 'Company' },
];

export const Default: Story = {
  args: { tabs, value: 'overview', children: text('Panel content for the selected tab.') },
};
// No `value`: a tablist always has exactly one selected tab, so the first one
// reads as selected — which is also what keeps the roving tabindex reachable.
export const Unselected: Story = {
  args: { tabs, children: text('Panel content for the selected tab.') },
};
export const TwoTabs: Story = {
  args: {
    tabs: [
      { value: 'open', label: 'Open' },
      { value: 'closed', label: 'Closed' },
    ],
    value: 'open',
    children: text('Panel content for the selected tab.'),
  },
};
