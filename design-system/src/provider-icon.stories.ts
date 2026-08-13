import type { Meta, StoryObj } from '@storybook/svelte';
import ProviderIcon from './provider-icon.svelte';

const meta = {
  title: 'Primitives/ProviderIcon',
  component: ProviderIcon,
  tags: ['autodocs'],
  argTypes: {
    provider: {
      control: 'select',
      options: ['google', 'github', 'telegram', 'linkedin', 'apple', 'discord'],
    },
  },
} satisfies Meta<typeof ProviderIcon>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Google: Story = { args: { provider: 'google' } };
export const Github: Story = { args: { provider: 'github' } };
export const Telegram: Story = { args: { provider: 'telegram' } };
export const Linkedin: Story = { args: { provider: 'linkedin' } };
export const Apple: Story = { args: { provider: 'apple' } };
export const Discord: Story = { args: { provider: 'discord' } };
