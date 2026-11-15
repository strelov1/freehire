<script lang="ts">
  import ExperienceBankView from '#lib/components/ExperienceBankView.svelte';

  // The workspace's Experience tab: the bank as its owner sees it on /my/profile.
  let { resetError, onBankMutated }: { resetError: string; onBankMutated: () => void } = $props();
</script>

<!-- The refresh a bank edit offers is the same reset the History tab's control runs,
     and it fails into the same `resetError` — but someone who triggered it from here
     is not looking at that tab, so the failure is said here too. -->
{#if resetError}
  <p class="mb-3 rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive-strong">
    {resetError}
  </p>
{/if}
<ExperienceBankView {onBankMutated} />
