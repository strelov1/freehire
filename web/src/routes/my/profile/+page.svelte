<script lang="ts">
  import { resumeStore } from '$lib/resume.svelte';
  import CvSummaryCard from '$lib/components/profile/CvSummaryCard.svelte';
  import ProfileForm from '$lib/components/ProfileForm.svelte';
  import RoleCard from '$lib/components/profile/RoleCard.svelte';
  import { profileStore } from '$lib/profile.svelte';
  import { handleCvDeleted, handleCvUploaded, handleSaved } from './actions';

  // The default profile section: CV upload/photo, the read-only CV summary, and Role.
  const profile = $derived(profileStore.profile);
  const resumeMeta = $derived(resumeStore.meta);
</script>

{#if profile}
  {#key profile.updated_at}
    <ProfileForm
      {profile}
      hasCv={resumeStore.present}
      uploadedAt={resumeMeta?.uploaded_at}
      onSaved={handleSaved}
      onCvUploaded={handleCvUploaded}
      onCvDeleted={handleCvDeleted}
    />
  {/key}

  <div class="mt-4">
    <CvSummaryCard
      structured={resumeMeta?.structured ?? null}
      contacts={resumeMeta?.contacts ?? null}
      onSaved={() => void resumeStore.refresh()}
    />
  </div>

  <div class="mt-4">
    <RoleCard {profile} onProfileChanged={handleSaved} />
  </div>
{/if}
