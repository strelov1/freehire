import type { Meta, StoryObj } from '@storybook/svelte';
// The component under test is Dialog; the demo supplies the trigger it needs to
// be reopenable. Same for the other snippet-composing primitives — see
// .storybook/demos/.
import DialogDemo from '../.storybook/demos/DialogDemo.svelte';

const meta = {
  title: 'Primitives/Dialog',
  component: DialogDemo,
  tags: ['autodocs'],
} satisfies Meta<typeof DialogDemo>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    title: 'Withdraw application?',
    description: 'The employer keeps the messages you already sent.',
  },
};
export const TitleOnly: Story = { args: { title: 'Withdraw application?' } };

// Held open: Escape, the backdrop and the close button all go away together,
// for the window in which an irreversible request is in flight and the dialog
// is the only place its outcome will appear. The in-dialog buttons are the way
// out, which is why they are not gated with the rest.
export const NotDismissible: Story = {
  args: {
    title: 'Deleting your account…',
    description: 'This cannot be undone, so the dialog holds until it finishes.',
    dismissible: false,
  },
};

// Resize the canvas below the sm breakpoint (640px) to see the mobile takeover:
// edge-to-edge, scrolling body, close button pinned to the viewport corner.
export const LongContent: Story = {
  args: { title: 'Update your profile', body: 'long' },
};
