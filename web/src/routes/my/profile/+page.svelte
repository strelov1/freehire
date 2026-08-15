<script lang="ts">
  import { Trash2 } from '@lucide/svelte';
  import { filtersFromProfile, filtersToParams } from '$lib/filters';
  import { savedSearches } from '$lib/savedSearches.svelte';
  import AccountPreferences from '$lib/components/AccountPreferences.svelte';
  import DeleteAccountButton from '$lib/components/DeleteAccountButton.svelte';
  import ProfileForm from '$lib/components/ProfileForm.svelte';
  import { profileStore } from '$lib/profile.svelte';
  import { resumeStore } from '$lib/resume.svelte';
  import { Button, ConfirmDialog } from '$lib/ui';

  // Settings is the profile's index route. The layout owns the load and renders the
  // first-time set-up form instead of this panel, so `profile` is non-null here.
  const profile = $derived(profileStore.profile);

  let actionError = $state<string | null>(null);
  let confirmRemoveOpen = $state(false);

  // Keep the profile-derived saved search (the "notify me about jobs matching my
  // profile" toggle, on the Search alerts page — ProfileAlertToggle) in step with a
  // changed role/skills/location — otherwise it would keep alerting on the profile as it
  // stood when first enabled. Best-effort: a failure here never blocks or rolls back the
  // profile save itself, it just leaves the alert stale until the next successful save.
  async function syncProfileAlert() {
    const p = profileStore.profile;
    if (!p) return;
    await savedSearches.ensureLoaded();
    const existing = savedSearches.items.find((s) => s.derived_from_profile);
    if (!existing) return;
    try {
      await savedSearches.update(existing.id, {
        query: filtersToParams(filtersFromProfile(p)).toString(),
      });
    } catch {
      // best-effort — see doc comment.
    }
  }

  async function remove() {
    actionError = null;
    try {
      await profileStore.clear();
    } catch {
      actionError = 'Could not delete the profile. Please try again.';
    }
  }
</script>

<svelte:head>
  <title>Profile settings — freehire</title>
</svelte:head>

{#if actionError}
  <p class="mb-4 text-sm text-destructive">{actionError}</p>
{/if}

{#if profile}
  {#key profile.updated_at}
    <ProfileForm
      {profile}
      hasCv={resumeStore.present}
      onSaved={() => void syncProfileAlert()}
      onCvUploaded={() => resumeStore.noteUpload()}
    />
  {/key}

  <AccountPreferences class="mt-6" />

  <!-- Destructive actions live at the foot of the settings section, out of the page
       header (where they crowded the title on narrow viewports) and off the other
       sections, which are readings of the market, not account settings. Both open
       behind a confirmation. -->
  <div class="mt-2 flex justify-end gap-2 border-t border-border pt-4">
    <Button
      variant="ghost"
      size="sm"
      onclick={() => (confirmRemoveOpen = true)}
      class="text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
    >
      <Trash2 class="size-4" />
      Delete profile
    </Button>
    <DeleteAccountButton />
  </div>
{/if}

<ConfirmDialog
  bind:open={confirmRemoveOpen}
  title="Delete your profile?"
  confirmLabel="Delete"
  variant="destructive"
  onConfirm={remove}
/>
