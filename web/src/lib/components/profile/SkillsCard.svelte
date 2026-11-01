<script lang="ts">
  // Skills autosave straight to the same profile, including an incomplete one.
  import { profileStore } from '#lib/profile.svelte.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, t } from '#lib/i18n/t.js';
  import { SaveStatus } from '#lib/saveStatus.svelte.js';
  import { messages } from './SkillsCard.messages';
  import SkillsPicker from './SkillsPicker.svelte';

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
  async function toggleSkill(skill: string) {
    if (status.saving) return;
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
  <SkillsPicker {skills} onToggleSkill={toggleSkill} {status} />

</div>
