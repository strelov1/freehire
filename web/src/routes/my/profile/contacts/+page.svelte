<script lang="ts">
  import CandidateContactsEditor from '$lib/components/CandidateContactsEditor.svelte';
  import { resumeStore } from '$lib/resume.svelte';

  // Editable contacts, parsed from (and kept in sync with) the stored CV. The layout
  // starts the résumé load; this section reads whatever it holds — a failed or
  // still-pending load leaves the editor on its empty fields, never an error.
  const meta = $derived(resumeStore.meta);
</script>

<svelte:head>
  <title>Profile details — freehire</title>
</svelte:head>

<CandidateContactsEditor
  contacts={meta?.contacts ?? null}
  parseStatus={meta?.parse_status ?? ''}
  parseDetail={meta?.parse_detail ?? ''}
  structurePending={Boolean(meta?.structure_pending)}
  onSaved={() => void resumeStore.refresh()}
/>
