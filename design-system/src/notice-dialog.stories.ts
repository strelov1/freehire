import type { Meta, StoryObj } from '@storybook/svelte';
// NoticeDialog composes Dialog and Button and needs a trigger to be
// reopenable — same reason Dialog and ConfirmDialog get one. See .storybook/demos/.
import NoticeDialogDemo from '../.storybook/demos/NoticeDialogDemo.svelte';

const meta = {
  title: 'Primitives/NoticeDialog',
  component: NoticeDialogDemo,
  tags: ['autodocs'],
} satisfies Meta<typeof NoticeDialogDemo>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    title: 'A new version of freehire is available',
    description: 'Reload to get it.',
    confirmLabel: 'Reload',
  },
};

export const TitleOnly: Story = {
  args: { title: 'Your changes were saved.' },
};
