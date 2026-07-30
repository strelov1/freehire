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
