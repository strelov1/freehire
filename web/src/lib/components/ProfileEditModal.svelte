<script lang="ts">
  import { ArrowUp } from '@lucide/svelte';
  import { ApiError, extractResumeSkills, facetCounts } from '$lib/api';
  import { emptyFilters, type FacetState, type JobFilters } from '$lib/filters';
  import { profileStore } from '$lib/profile.svelte';
  import type { StagedFilters } from '$lib/stagedFilters.svelte';
  import type { FacetCounts, UserProfile } from '$lib/types';
  import FilterModal from './filters/FilterModal.svelte';

  // Edit the single profile with the SAME modal as the job filters (FilterModal),
  // restricted to the Specialization + Skills panes and with a Save action instead of
  // "Show results". Seeded from the current profile; a CV-upload extra merges extracted
  // skills into the staged skills.
  let { profile, onClose }: { profile: UserProfile | null; onClose: () => void } = $props();

  // Seed the modal from the current profile's specialization + skill sets. The modal is
  // recreated on every open, so capturing the initial `profile` value is intended.
  const seed: JobFilters = emptyFilters();
  const facet = (values: string[]): FacetState => ({ values: [...values], exclude: false, matchAll: false });
  // svelte-ignore state_referenced_locally
  seed.facets['category'] = facet(profile?.specializations ?? []);
  // svelte-ignore state_referenced_locally
  seed.facets['skills'] = facet(profile?.skills ?? []);

  // Only the Specialization + Skills panes; hide Seniority (rendered inside the category
  // pane) — a profile is specializations + skills, not seniority.
  const railKeys = ['category', 'skills'];
  const exclude = ['seniority'];

  // Live skill distribution for the dynamic skills control (same source the job filters use).
  let counts = $state<FacetCounts | null>(null);
  $effect(() => {
    facetCounts(new URLSearchParams())
      .then((c) => (counts = c))
      .catch(() => {});
  });

  // Save is allowed only with at least one specialization and one skill (the server's
  // 5-specialization cap is enforced there and surfaced as the modal's apply error).
  const canApply = (f: JobFilters) =>
    (f.facets.category?.values.length ?? 0) > 0 && (f.facets.skills?.values.length ?? 0) > 0;

  async function save(staged: StagedFilters) {
    await profileStore.save(staged.facet('category').values, staged.facet('skills').values);
  }

  // CV upload → skill extraction, merged (union) into the staged skills.
  let resumeBusy = $state(false);
  let resumeError = $state<string | null>(null);
  let resumeNote = $state<string | null>(null);
  let fileInput = $state<HTMLInputElement | null>(null);
  let dragActive = $state(false);

  async function analyzeResume(staged: StagedFilters, file: File) {
    resumeBusy = true;
    resumeError = null;
    resumeNote = null;
    try {
      const extracted = await extractResumeSkills(file);
      if (extracted.length === 0) {
        resumeNote = 'No known skills found in the CV.';
        return;
      }
      const before = staged.facet('skills').values.length;
      for (const s of extracted) staged.add('skills', s);
      const added = staged.facet('skills').values.length - before;
      resumeNote =
        added > 0
          ? `Added ${added} skill${added === 1 ? '' : 's'} from your CV.`
          : 'All CV skills were already listed.';
    } catch (err) {
      resumeError = err instanceof ApiError ? err.message : 'Could not read the CV. Please try again.';
    } finally {
      resumeBusy = false;
    }
  }

  function onResumeFile(staged: StagedFilters, e: Event) {
    const target = e.currentTarget as HTMLInputElement;
    const file = target.files?.[0];
    target.value = ''; // clear so re-selecting the same file fires change again
    if (file) void analyzeResume(staged, file);
  }

  function isPdf(file: File): boolean {
    return file.type === 'application/pdf' || file.name.toLowerCase().endsWith('.pdf');
  }

  function onDrop(staged: StagedFilters, e: DragEvent) {
    e.preventDefault();
    dragActive = false;
    const file = e.dataTransfer?.files?.[0];
    if (!file) return;
    if (!isPdf(file)) {
      resumeError = 'Please drop a PDF file.';
      return;
    }
    void analyzeResume(staged, file);
  }
</script>

{#snippet cvUpload(staged: StagedFilters)}
  <div class="flex flex-col gap-1.5">
    <span class="text-sm font-medium">Add skills from your CV</span>
    <input
      type="file"
      accept="application/pdf,.pdf"
      class="hidden"
      bind:this={fileInput}
      onchange={(e) => onResumeFile(staged, e)}
    />
    <button
      type="button"
      onclick={() => fileInput?.click()}
      ondragover={(e) => {
        e.preventDefault();
        dragActive = true;
      }}
      ondragleave={(e) => {
        e.preventDefault();
        dragActive = false;
      }}
      ondrop={(e) => onDrop(staged, e)}
      disabled={resumeBusy}
      class="flex items-center justify-center gap-2 rounded-xl border-2 border-dashed px-4 py-3 text-center text-sm transition-colors disabled:opacity-70 {dragActive
        ? 'border-primary bg-primary/5'
        : 'border-border hover:border-primary/60'}"
    >
      <ArrowUp class="size-4 shrink-0 text-primary" />
      <span class="font-medium">{resumeBusy ? 'Analyzing…' : 'Drop a PDF or choose a file'}</span>
    </button>
    {#if resumeError}
      <p class="text-sm text-destructive">{resumeError}</p>
    {:else if resumeNote}
      <p class="text-xs text-muted-foreground">{resumeNote}</p>
    {/if}
  </div>
{/snippet}

<FilterModal
  {seed}
  {counts}
  {railKeys}
  {exclude}
  {canApply}
  plain
  title="Edit profile"
  applyLabel="Save"
  onApply={save}
  open
  {onClose}
  extra={cvUpload}
/>
