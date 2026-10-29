<script lang="ts">
  // The Skills view: wraps SkillsPicker with autosave straight to profileStore — the
  // steady-state (profile already exists) counterpart to the local, batched skills fields
  // ProfileForm still uses during first-time set-up.
  import { profileStore } from '#lib/profile.svelte.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, t } from '#lib/i18n/t.js';
  import { SaveStatus } from '#lib/saveStatus.svelte.js';
  import { messages } from './SkillsCard.messages';
  import SkillsPicker from './SkillsPicker.svelte';
  import { announce } from '#lib/announce.svelte.js';

  const s = $derived(t(messages, locale()));

  let {
    onProfileChanged,
  }: {
    /** Fired after a skill change saves — the profile-derived saved-search alert needs
     *  to stay in step with it, the same way RoleCard's and LocationCard's own
     *  autosaves already do. */
    onProfileChanged?: () => void;
  } = $props();

  // No local buffer: skills read straight off the profile, so a write's effect on the chips
  // comes purely from the store's own reactive state changing.
  const skills = $derived(profileStore.profile?.skills ?? []);

  // One write in flight at a time (mirrors JobMatch.svelte): a second toggle started before
  // the first settles would race against a profile row it hasn't seen yet.
  const status = new SaveStatus();
  // Set instead of attempting a doomed write: the server requires at least one skill on
  // every save, not just at creation (normalizeSkills in internal/userprofile), so removing
  // the one skill left would always 400.
  let lastSkillBlocked = $state(false);

  const isOnlySkill = (skill: string) => skills.length === 1 && skills.includes(skill);

  async function toggleSkill(skill: string) {
    if (status.saving) return;
    if (skills.includes(skill) && isOnlySkill(skill)) {
      lastSkillBlocked = true;
      announce(s.lastSkillBlocked);
      return;
    }
    lastSkillBlocked = false;
    const removing = skills.includes(skill);
    await status.run(
      async () => {
        await (removing ? profileStore.removeSkill(skill) : profileStore.addSkill(skill));
        onProfileChanged?.();
      },
      { saved: format(removing ? s.removed : s.added, { skill }), failed: format(s.saveFailed, { skill }) },
    );
  }
</script>

<div class="flex flex-col gap-4">
  <SkillsPicker {skills} onToggleSkill={toggleSkill} busy={status.saving} {status} />

  {#if lastSkillBlocked}
    <p class="text-sm text-muted-foreground">{s.lastSkillBlocked}</p>
  {/if}
</div>
