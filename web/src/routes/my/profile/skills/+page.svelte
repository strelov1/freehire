<script lang="ts">
  import SkillsCard from '$lib/components/profile/SkillsCard.svelte';
  import { handleSaved } from '../actions';
</script>

<SkillsCard onProfileChanged={handleSaved} />
