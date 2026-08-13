import type { Meta, StoryObj } from '@storybook/svelte';
import TabStripDemo from '../.storybook/demos/TabStripDemo.svelte';

const meta = {
  title: 'Primitives/TabStrip',
  component: TabStripDemo,
  tags: ['autodocs'],
} satisfies Meta<typeof TabStripDemo>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Fitting: Story = {
  args: {
    tabs: [
      { id: 'overview', label: 'Overview' },
      { id: 'experience', label: 'Experience' },
      { id: 'skills', label: 'Skills' },
    ],
  },
};

// Enough tabs to overflow a `max-w-md` strip, so the horizontal scroll and the
// fade mask on the trailing edge are both visible.
export const Overflowing: Story = {
  args: {
    tabs: [
      { id: 'overview', label: 'Overview' },
      { id: 'experience', label: 'Experience' },
      { id: 'projects', label: 'Projects' },
      { id: 'skills', label: 'Skills' },
      { id: 'education', label: 'Education' },
      { id: 'certifications', label: 'Certifications' },
      { id: 'references', label: 'References' },
      { id: 'cv-readiness', label: 'CV readiness' },
    ],
  },
};
