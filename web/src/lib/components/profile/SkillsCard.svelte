<script lang="ts">
  // The Skills view: wraps SkillsPicker with autosave straight to profileStore — the
  // steady-state (profile already exists) counterpart to the local, batched skills fields
  // ProfileForm still uses during first-time set-up.
  import { profileStore } from '$lib/profile.svelte';
  import SkillsPicker from './SkillsPicker.svelte';

  // No local buffer: both lists read straight off the profile, so a write's effect on the
  // chips comes purely from the store's own reactive state changing.
  const skills = $derived(profileStore.profile?.skills ?? []);
  const excludedSkills = $derived(profileStore.profile?.excluded_skills ?? []);

  // One write in flight at a time (mirrors JobMatch.svelte): a second toggle started before
  // the first settles would race against a profile row it hasn't seen yet.
  let pending = $state(false);
  let failed = $state<string | null>(null);
  // Set instead of attempting a doomed write: the server requires at least one skill on
  // every save, not just at creation (normalizeSkills in internal/userprofile), so removing
  // — or avoiding, which also un-claims — the one skill left would always 400.
  let lastSkillBlocked = $state(false);

  const isOnlySkill = (skill: string) => skills.length === 1 && skills.includes(skill);

  async function toggleSkill(skill: string) {
    if (pending) return;
    if (skills.includes(skill) && isOnlySkill(skill)) {
      lastSkillBlocked = true;
      return;
    }
    pending = true;
    failed = null;
    lastSkillBlocked = false;
    try {
      await (skills.includes(skill) ? profileStore.removeSkill(skill) : profileStore.addSkill(skill));
    } catch {
      failed = skill;
    } finally {
      pending = false;
    }
  }

  async function toggleExcludedSkill(skill: string) {
    if (pending) return;
    if (!excludedSkills.includes(skill) && isOnlySkill(skill)) {
      lastSkillBlocked = true;
      return;
    }
    pending = true;
    failed = null;
    lastSkillBlocked = false;
    try {
      await (excludedSkills.includes(skill)
        ? profileStore.unavoidSkill(skill)
        : profileStore.avoidSkill(skill));
    } catch {
      failed = skill;
    } finally {
      pending = false;
    }
  }
</script>

<div class="flex flex-col gap-4">
  <SkillsPicker {skills} {excludedSkills} onToggleSkill={toggleSkill} onToggleExcluded={toggleExcludedSkill} busy={pending} />

  {#if lastSkillBlocked}
    <p class="text-sm text-muted-foreground">You need at least one skill — add another before removing or avoiding this one.</p>
  {:else if failed}
    <p class="text-sm text-destructive">Could not update {failed} in your profile. Try again.</p>
  {/if}
</div>
