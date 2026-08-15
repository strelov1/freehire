import type { Meta, StoryObj } from '@storybook/svelte';
import EmailPreview from './email-preview.svelte';

/**
 * Every transactional email freehire sends, rendered by the Go templates that
 * actually send them and framed here for review.
 *
 * These stories live outside the primitives on purpose: an email is not a
 * component of this package. It is another surface that must look like the same
 * product, and Storybook is where the product's surfaces are looked at.
 *
 * The files come from `go run ./cmd/mail-preview` at the repo root. Gallery reads
 * the generated index, so a newly added mail shows up there without editing this
 * file; the individual stories below are hand-listed for direct linking.
 */
const meta = {
  title: 'Email/Templates',
  component: EmailPreview,
  parameters: { layout: 'centered' },
  // The theme toolbar in the top bar switches the docs chrome, not a framed
  // document, so the scheme is a control on the story instead: pick Light or Dark
  // in the Controls panel to compare, or Auto to see what your own client would do.
  argTypes: {
    scheme: { control: 'inline-radio', options: ['light', 'dark', 'auto'] },
  },
  args: { scheme: 'light' },
} satisfies Meta<typeof EmailPreview>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The contact sheet: all mails side by side, generated from the Go sample list. */
export const Gallery: Story = { args: { name: 'index', height: 900 } };

export const VerifyEmail: Story = { args: { name: 'verify-email', height: 520 } };
export const PasswordReset: Story = { args: { name: 'password-reset', height: 520 } };
export const SubscriptionDigest: Story = { args: { name: 'subscription-digest', height: 640 } };
export const SavedJobReminder: Story = { args: { name: 'saved-job-reminder', height: 480 } };
export const NudgeFollowUp: Story = { args: { name: 'nudge-follow-up', height: 480 } };
export const NudgeJobClosed: Story = { args: { name: 'nudge-job-closed', height: 480 } };
export const ReferralRequest: Story = { args: { name: 'referral-request', height: 480 } };
export const ReportJobRemoved: Story = { args: { name: 'report-job-removed', height: 700 } };
export const ReportDismissed: Story = { args: { name: 'report-dismissed', height: 640 } };
