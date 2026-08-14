import type { Meta, StoryObj } from '@storybook/svelte';
// ConfirmDialog composes Dialog and Button and needs a trigger to be
// reopenable — same reason Dialog itself gets a demo. See .storybook/demos/.
import ConfirmDialogDemo from '../.storybook/demos/ConfirmDialogDemo.svelte';

const meta = {
  title: 'Primitives/ConfirmDialog',
  component: ConfirmDialogDemo,
  tags: ['autodocs'],
} satisfies Meta<typeof ConfirmDialogDemo>;

export default meta;
type Story = StoryObj<typeof meta>;

// A reversible removal: the button under the confirm dialog title reads
// 'primary', not the filled destructive red — that stays for the one below.
export const Default: Story = {
  args: {
    title: 'Delete saved search "Remote Go roles"?',
    confirmLabel: 'Delete',
    variant: 'primary',
  },
};

export const WithDescription: Story = {
  args: {
    title: 'Disconnect Telegram?',
    description: 'You will stop receiving alerts.',
    confirmLabel: 'Disconnect',
    variant: 'primary',
  },
};

// Severe and irreversible — the only case that earns the filled destructive button.
export const Destructive: Story = {
  args: {
    title: 'Delete your profile?',
    description: 'This cannot be undone.',
    confirmLabel: 'Delete',
    variant: 'destructive',
  },
};

// onConfirm rejects: busy shows on the confirm button while it runs, then the
// dialog stays open with the thrown message in place instead of vanishing.
export const ConfirmFails: Story = {
  args: {
    title: 'Revoke "CI deploy key"?',
    description: 'Any script using it stops working immediately.',
    confirmLabel: 'Revoke',
    variant: 'destructive',
    fails: true,
  },
};
