<script lang="ts">
  import MentorSessionDetail from '#lib/components/MentorSessionDetail.svelte';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();
</script>

<MentorSessionDetail session={data.session} asMentor={true} />
