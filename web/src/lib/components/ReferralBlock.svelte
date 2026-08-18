<script lang="ts">
  import { Handshake } from '@lucide/svelte';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { Button } from '$lib/ui';

  // Shown on a vacancy and on the company page when the company has an approved referrer.
  // jobId is the optional source-vacancy context passed through to the request.
  let {
    companySlug,
    companyName,
    jobId,
  }: {
    companySlug: string;
    companyName: string;
    jobId?: number;
  } = $props();

  // The request form is imported on the click that opens it, not with the page.
  // This block itself is small, but the modal behind it pulls in Dialog, FormField
  // and the whole request flow — and it was landing in the route's module graph for
  // every job and company page, which is every visitor paying for a form that only
  // the few who actually ask for a referral ever open. Same lazy posture as shiki
  // and easymde elsewhere.
  //
  // `$state.raw`: this holds a component constructor, which must be stored as-is —
  // a deep proxy around it is both pointless and a hazard.
  type RequestReferralModal = typeof import('./RequestReferralModal.svelte').default;
  let Modal = $state.raw<RequestReferralModal | null>(null);
  let loading = $state(false);
  let open = $state(false);

  async function ask() {
    if (!isAuthenticated()) {
      openAuthDialog();
      return;
    }
    if (!Modal) {
      // The button is disabled for the duration: on a slow connection the chunk is
      // a visible wait, and a click that looks ignored invites a second one.
      loading = true;
      try {
        Modal = (await import('./RequestReferralModal.svelte')).default;
      } catch {
        // Offline or a failed deploy swap — leave the button live so a retry works.
        return;
      } finally {
        loading = false;
      }
    }
    open = true;
  }
</script>

<section
  class="flex items-center gap-4 rounded-lg border border-brand/25 bg-brand-muted px-4 py-3.5"
>
  <Handshake class="size-6 shrink-0 text-brand-strong" />
  <div class="min-w-0 flex-1">
    <h3 class="text-sm font-semibold text-brand-strong">Referral available at {companyName}</h3>
    <p class="text-xs text-brand-strong/80">
      An employee here can refer you. The referrer stays anonymous and reaches out to you
      directly if interested.
    </p>
  </div>
  <Button variant="primary" size="sm" onclick={ask} disabled={loading}>Ask for a referral</Button>
</section>

{#if open && Modal}
  <Modal {companySlug} {companyName} {jobId} onClose={() => (open = false)} />
{/if}
