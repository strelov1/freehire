<script lang="ts">
  import MentorScheduleEditor from '$lib/components/MentorScheduleEditor.svelte';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  // Local copy so the editor can write back without a round trip through `load`.
  let availability = $state(data.availability);
</script>

<MentorScheduleEditor bind:rules={availability} />
