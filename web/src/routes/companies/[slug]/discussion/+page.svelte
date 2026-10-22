<script lang="ts">
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import { promptSignIn } from '#lib/signin.js';
  import CompanyFeedbackDialog from '#lib/components/CompanyFeedbackDialog.svelte';
  import CompanyHeader from '#lib/components/CompanyHeader.svelte';
  import CompanyTabs from '#lib/components/CompanyTabs.svelte';
  import DiscussionIndex from '#lib/components/community/DiscussionIndex.svelte';
  import SubjectHeader from '#lib/components/community/SubjectHeader.svelte';
  import { Breadcrumbs, Button } from '#lib/ui/index.js';

  let { data } = $props();

  // The discussion is the company page's third tab — but only while the company is
  // listed under THIS slug. A discussion outlives its company, and a merged company's
  // threads stay at the retired slug (they store it), so the canonical company's tabs
  // would point at a different thread list. Those keep the header-less layout.
  const company = $derived(data.company?.slug === data.slug ? data.company : null);

  // Feedback is a rated review, distinct from the anonymous discussion below it —
  // it needs a signed-in caller up front rather than deferring to the report dialog,
  // the same gate JobView uses before its own.
  let showFeedback = $state(false);
  function openFeedback() {
    if (!isAuthenticated()) {
      promptSignIn();
      return;
    }
    showFeedback = true;
  }
</script>

{#if company}
  <div class="mx-auto w-full max-w-6xl px-4 py-6">
    <Breadcrumbs
      items={[{ name: 'Companies', href: '/companies' }, { name: company.name, href: `/companies/${company.slug}` }, { name: 'Discussion' }]}
      class="mb-4"
    />
    {#key data.slug}
      <CompanyHeader {company} slug={data.slug} />
      <CompanyTabs slug={data.slug} active="discussion">
        <div class="mt-6 max-w-3xl">
          <DiscussionIndex
            subjectType="company"
            subjectSlug={data.slug}
            initialThreads={data.threads}
            initialCursor={data.nextCursor}
          />
        </div>
      </CompanyTabs>
    {/key}
  </div>
{:else}
  <div class="mx-auto w-full max-w-3xl px-4 py-6">
    <!-- The header is a link and the feedback button is an action, so they are
         siblings: nesting a button inside the link would swallow its click. -->
    <div class="mb-4 flex items-center gap-2">
      <SubjectHeader
        subject={data.subject}
        absence={data.absence}
        subjectType="company"
        subjectSlug={data.slug}
        class="min-w-0 flex-1"
      />
      <Button variant="outline" class="shrink-0" onclick={openFeedback}>Leave feedback</Button>
    </div>
    <DiscussionIndex
      subjectType="company"
      subjectSlug={data.slug}
      initialThreads={data.threads}
      initialCursor={data.nextCursor}
    />
  </div>

  {#if showFeedback}
    <CompanyFeedbackDialog slug={data.slug} onClose={() => (showFeedback = false)} />
  {/if}
{/if}
