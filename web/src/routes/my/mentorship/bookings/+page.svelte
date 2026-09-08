<script lang="ts">
  import MentorSessionSplit from '$lib/components/MentorSessionSplit.svelte';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();
</script>

<MentorSessionSplit sessions={data.bookings} empty="Nobody has booked yet." />
