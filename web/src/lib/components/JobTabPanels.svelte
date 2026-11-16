<script lang="ts">
  import { tabStripId } from '#lib/ui/index.js';
  import type { Display } from '#lib/generated/contracts.js';
  import type { JobContentTabs } from './jobContentTabs.svelte';
  import type { Job } from '#lib/types.js';
  import JobApplyForm from './JobApplyForm.svelte';
  import JobCompanyPanel from './JobCompanyPanel.svelte';
  import JobDiscussionPanel from './JobDiscussionPanel.svelte';
  import JobPostingBody from './JobPostingBody.svelte';

  // The job page's tab panels: description, company, application form, discussion.
  let {
    panelId,
    tabs,
    job,
    lang,
    applyForm,
  }: { panelId: string; tabs: JobContentTabs; job: Job; lang: string | undefined; applyForm: Display | null } =
    $props();
</script>

<!-- One panel for every tab, its contents toggled by class rather than {#if}.
     Unmounting the inactive one would throw away the company the visitor already
     waited for, and re-render the description on every switch back. It also keeps
     every panel in the server-rendered HTML, so a crawler reads what a visitor
     would have to click for. -->
<div id={panelId} role="tabpanel" aria-labelledby={tabStripId(panelId, tabs.current)}>
  <div class={tabs.current === 'description' ? 'flex flex-col gap-6' : 'hidden'}>
    <JobPostingBody {job} {lang} />
  </div>
  {#if tabs.companyWithDetails}
    <div class={tabs.current === 'company' ? 'block' : 'hidden'}>
      {#key tabs.companySlug}
        <JobCompanyPanel
          company={tabs.companyWithDetails}
          slug={tabs.companySlug}
          name={job.company || 'this company'}
        />
      {/key}
    </div>
  {/if}
  <div class={tabs.current === 'application' ? 'block' : 'hidden'}>
    <JobApplyForm form={applyForm} />
  </div>
  <div class={tabs.current === 'discussion' ? 'block' : 'hidden'}>
    {#key job.public_slug}
      <JobDiscussionPanel slug={job.public_slug} active={tabs.current === 'discussion'} />
    {/key}
  </div>
</div>
