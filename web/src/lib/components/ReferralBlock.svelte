<script lang="ts">
  import { Handshake } from '@lucide/svelte';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { Button } from '$lib/ui';
  import RequestReferralModal from './RequestReferralModal.svelte';

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

  let open = $state(false);

  function ask() {
    if (!isAuthenticated()) {
      openAuthDialog();
      return;
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
  <Button variant="primary" size="sm" onclick={ask}>Ask for a referral</Button>
</section>

{#if open}
  <RequestReferralModal
    {companySlug}
    {companyName}
    {jobId}
    onClose={() => (open = false)}
  />
{/if}
