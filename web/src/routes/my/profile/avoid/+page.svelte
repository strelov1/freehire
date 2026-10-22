<script lang="ts">
  import AvoidCard from '#lib/components/profile/AvoidCard.svelte';
  import { handleSaved } from '../actions';
</script>

<AvoidCard onProfileChanged={handleSaved} />
