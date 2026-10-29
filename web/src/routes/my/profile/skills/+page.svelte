<script lang="ts">
  import CompletenessBadge from '#lib/components/profile/CompletenessBadge.svelte';
  import SkillsCard from '#lib/components/profile/SkillsCard.svelte';
  import { handleSaved } from '../actions';
</script>

<div class="mb-4">
  <CompletenessBadge section="skills" />
</div>

<SkillsCard onProfileChanged={handleSaved} />
