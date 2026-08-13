import type { Meta, StoryObj } from '@storybook/svelte';
import AvatarFallbackIconDemo from '../.storybook/demos/AvatarFallbackIconDemo.svelte';

const meta = {
  title: 'Primitives/Avatar (fallback icon)',
  component: AvatarFallbackIconDemo,
  tags: ['autodocs'],
  argTypes: {
    size: { control: 'select', options: ['sm', 'md', 'lg'] },
  },
} satisfies Meta<typeof AvatarFallbackIconDemo>;

export default meta;
type Story = StoryObj<typeof meta>;

// No name and no photo — the icon is the entity's only mark, e.g. a company with
// neither a resolved logo nor a name string to derive initials from.
export const Default: Story = { args: { size: 'md' } };
