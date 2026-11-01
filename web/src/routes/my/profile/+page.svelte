<script lang="ts">
  import { MapPin } from '@lucide/svelte';
  import { resumeStore } from '#lib/resume.svelte.js';
  import AccountTimezone from '#lib/components/AccountTimezone.svelte';
  import CandidateContactsEditor from '#lib/components/CandidateContactsEditor.svelte';
  import CvSummaryCard from '#lib/components/profile/CvSummaryCard.svelte';
  import LocationCard from '#lib/components/profile/LocationCard.svelte';
  import ProfileForm from '#lib/components/ProfileForm.svelte';
  import RoleCard from '#lib/components/profile/RoleCard.svelte';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { profileStore } from '#lib/profile.svelte.js';
  import { SaveStatus } from '#lib/saveStatus.svelte.js';
  import SaveStatusLine from '#lib/components/SaveStatus.svelte';
  import { handleCvDeleted, handleCvUploaded, handleSaved } from './actions';
  import { messages } from './messages';

  // The default profile section: who the candidate is and where. CV upload/photo, the
  // read-only CV summary and Role, then Contacts and Location, which used to be tabs of
  // their own (their old routes redirect to the anchors below).
  const s = $derived(t(messages, locale()));
  const profile = $derived(profileStore.profile);
  const resumeMeta = $derived(resumeStore.meta);
  const locationStatus = new SaveStatus();
</script>

{#if profile}
  <ProfileForm
    {profile}
    hasCv={resumeStore.present}
    uploadedAt={resumeMeta?.uploaded_at}
    onSaved={handleSaved}
    onCvUploaded={handleCvUploaded}
    onCvDeleted={handleCvDeleted}
  />

  <div class="mt-4">
    <CvSummaryCard structured={resumeMeta?.structured ?? null} />
  </div>

  <div class="mt-4">
    <RoleCard {profile} onProfileChanged={handleSaved} />
  </div>

  <div id="account-contacts" class="mt-8 scroll-mt-20 border-t border-border pt-6">
    <CandidateContactsEditor contacts={resumeMeta?.contacts ?? null} />
  </div>

  <!-- `account-location` is the target of the setup checklist's location step (see
       accountCompleteness.ts). LocationPreferencesFields seeds its local edit state once
       from `profile`, on the explicit contract that the caller remounts it on a genuinely
       different value. profileStore.refresh() (CV delete/upload) can replace `profile`
       while this view is open, so key it on the store's re-reads — not on `updated_at`,
       which the card's own autosave also moves, and remounting on that reset a base-city
       search mid-edit. -->
  <section id="account-location" class="mt-8 flex scroll-mt-20 flex-col gap-4 border-t border-border pt-6">
    <div class="flex items-baseline justify-between gap-3">
      <h2 class="flex items-center gap-2 text-base font-semibold">
        <MapPin class="size-4.5" />{s.headings.location}
      </h2>
      <SaveStatusLine status={locationStatus} />
    </div>
    {#key profileStore.reads}
      <LocationCard {profile} status={locationStatus} onProfileChanged={handleSaved} />
    {/key}

    <!-- The timezone belongs with where the candidate is: the Talent Network card shows it,
         and digests and quiet hours are read in it. Outside the `{#key}` above, so a profile
         refresh never remounts it mid-pick. `account-timezone` is the target of the
         notification settings' "set your timezone" hint. -->
    <div id="account-timezone" class="mt-4 scroll-mt-20">
      <AccountTimezone />
    </div>
  </section>
{/if}
