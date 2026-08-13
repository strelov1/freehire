<script lang="ts">
  import { resolve } from '$app/paths';
  import { MessageSquare, Star } from '@lucide/svelte';
  import { api } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import type { Company, CompanyFeedbackSummary } from '$lib/types';
  import CompanyFeedbackDialog from './CompanyFeedbackDialog.svelte';
  import CompanyFeedbackListDialog from './CompanyFeedbackListDialog.svelte';
  import { companyLogoUrl } from '$lib/logo';
  import { EntityLogo } from '$lib/ui';
  import CredentialBadge from './CredentialBadge.svelte';
  import BackerBadge from './BackerBadge.svelte';
  import CompanyFollowButton from './CompanyFollowButton.svelte';
  import VoteControl from './VoteControl.svelte';

  // The company's header card: identity (logo + name + tagline + follow CTA) with the
  // industries and the website/LinkedIn links below a divider. The scalar company facts
  // live in a separate CompanyFacts card (jobs sidebar on desktop, under the header on
  // mobile), so this stays compact. The card is always shown, even for an unenriched
  // company (just the identity row) — the tagline and the meta divider are present-only.
  let { company, slug }: { company: Company; slug: string } = $props();

  // Open-thread count for the "Discussion · N" badge; loaded client-side so the
  // header renders immediately and the number fills in. Failures leave it hidden.
  let threadCount = $state<number | null>(null);
  $effect(() => {
    let alive = true;
    api
      .countThreads('company', slug)
      .then((n) => alive && (threadCount = n))
      .catch(() => {});
    return () => {
      alive = false;
    };
  });

  // The feedback list dialog (star ratings + category + text), opened from the
  // rating badge below. Before any feedback exists there's nothing to list, so the
  // same slot instead opens the write dialog directly — the header always carries
  // one feedback entry point, not just once a rating exists to show. Once the list
  // is open, its own "Write feedback" button is the way back into the write dialog.
  //
  // feedbackCount/feedbackRatingAvg are derived from the server-rendered `company`
  // prop (so a company switch or refreshed data is picked up, the same as
  // threadCount reacting to `slug`), overridden only after a save: the dialog's
  // `onSaved` already carries the write's freshly recomputed counters (Upsert
  // and Delete both return them), so the badge flips immediately with no
  // follow-up fetch — and so no window where that fetch can itself fail and
  // leave the badge stale, which is what the earlier getCompany-refetch version
  // of this function could do.
  let showFeedbackList = $state(false);
  let showFeedbackForm = $state(false);
  let feedbackOverride = $state<CompanyFeedbackSummary | null>(null);
  // A save applies to one company only; drop a stale override on a client-side
  // switch to another company (SvelteKit reuses this component across a
  // `/companies/[slug]` param change) — otherwise the previous company's
  // reviewed-just-now counters would keep showing on the new one.
  $effect(() => {
    void slug;
    feedbackOverride = null;
  });
  const feedbackCount = $derived(feedbackOverride?.feedback_count ?? company.feedback_count);
  const feedbackRatingAvg = $derived(feedbackOverride?.feedback_rating_avg ?? company.feedback_rating_avg);
  function openFeedbackEntry() {
    if (feedbackCount > 0) {
      showFeedbackList = true;
      return;
    }
    openFeedbackForm();
  }
  function openFeedbackForm() {
    if (!isAuthenticated()) {
      openAuthDialog('login');
      return;
    }
    showFeedbackList = false;
    showFeedbackForm = true;
  }

  const info = $derived(company.company_info ?? {});
  const industries = $derived(company.industries ?? []);
  // The homepage lives under `website` (hirebase) or `homepage` (older records) — read
  // whichever is present so the link renders regardless of which importer filled it.
  const website = $derived(info.website ?? info.homepage);
  const websiteHref = $derived(website ? (website.startsWith('http') ? website : `https://${website}`) : '');
  const linkedin = $derived(info.linkedin);

  const hasMeta = $derived(industries.length > 0 || !!website || !!linkedin);
</script>

<section class="rounded-2xl border border-border bg-card p-5">
  <div class="flex items-start gap-3">
    <EntityLogo name={company.name} src={companyLogoUrl(company.name) ?? undefined} shape="square" size="lg" />
    <div class="min-w-0 flex-1">
      <!-- break-words: a long single word ("International") is wider than the phone
           column and would otherwise overflow the card instead of wrapping. -->
      <h1 class="break-words text-xl font-semibold tracking-tight sm:text-2xl">{company.name}</h1>
      {#if company.tagline}
        <p class="mt-1 text-sm text-muted-foreground">{company.tagline}</p>
      {/if}
      <!-- Who backed the company, then the register-backed credentials (visa-sponsor
           licences) — both facts about the employer, so both sit under the identity.
           Backers lead: "picked by Y Combinator" is what a reader came to know.
           Each credential carries its issuing body and the disclaimer that the
           licence is not a promise about any given role. -->
      {#if company.collections?.length}
        <div class="mt-2 flex flex-wrap items-center gap-1.5">
          <BackerBadge collections={company.collections} withLabel />
          <CredentialBadge collections={company.collections} />
        </div>
      {/if}
    </div>
    <!-- flex-1 above lets the name/tagline use the full width; the follow CTA stays
         top-right and never shrinks (it collapses to an icon on mobile itself). -->
    <div class="shrink-0">
      <CompanyFollowButton {slug} companyName={company.name} />
    </div>
  </div>
  <!-- Discussion + votes get their own right-aligned row instead of sharing the
       identity row: together they are ~220px of non-shrinking width, which on a phone
       would leave the name/tagline column too narrow to hold a word. -->
  <div class="mt-3 flex flex-wrap items-center justify-end gap-x-4 gap-y-2">
    <a
      class="inline-flex items-center gap-1.5 text-sm font-medium text-primary hover:underline"
      href={resolve('/companies/[slug]/discussion', { slug })}
    >
      <MessageSquare class="size-4" aria-hidden="true" /> Discussion{threadCount
        ? ` · ${threadCount}`
        : ''}
    </a>
    <button
      type="button"
      class="inline-flex items-center gap-1.5 text-sm font-medium text-primary hover:underline"
      onclick={openFeedbackEntry}
    >
      {#if feedbackCount > 0}
        <Star class="size-4" fill="currentColor" aria-hidden="true" />
        {(feedbackRatingAvg ?? 0).toFixed(1)} · {feedbackCount}
      {:else}
        <Star class="size-4" aria-hidden="true" />
        Leave feedback
      {/if}
    </button>
    <VoteControl
      target="company"
      {slug}
      upvoteCount={company.upvote_count ?? 0}
      downvoteCount={company.downvote_count ?? 0}
      myVote={company.my_vote ?? 0}
    />
  </div>
  {#if hasMeta}
    <!-- Industries and links are two groups, not one flat wrap list: on a phone a flat
         list mixes a chip and a URL on the same line. Grouping keeps them on one row
         on desktop (both groups fit) and wraps them as whole blocks on mobile. -->
    <div class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-2 border-t border-border pt-3">
      {#if industries.length}
        <div class="flex flex-wrap items-center gap-2">
          {#each industries as industry (industry)}
            <span class="rounded-full bg-secondary px-2.5 py-0.5 text-xs text-secondary-foreground">{industry}</span>
          {/each}
        </div>
      {/if}
      {#if website || linkedin}
        <div class="flex min-w-0 flex-wrap items-center gap-x-4 gap-y-2">
          {#if website}
            <!-- eslint-disable svelte/no-navigation-without-resolve -- external company website, not an internal route -->
            <a
              class="min-w-0 break-all text-sm font-medium text-primary hover:underline"
              href={websiteHref}
              target="_blank"
              rel="noopener noreferrer">{website} ↗</a
            >
            <!-- eslint-enable svelte/no-navigation-without-resolve -->
          {/if}
          {#if linkedin}
            <!-- eslint-disable svelte/no-navigation-without-resolve -- external LinkedIn URL, not an internal route -->
            <a
              class="text-sm font-medium text-primary hover:underline"
              href={linkedin}
              target="_blank"
              rel="noopener noreferrer">LinkedIn ↗</a
            >
            <!-- eslint-enable svelte/no-navigation-without-resolve -->
          {/if}
        </div>
      {/if}
    </div>
  {/if}
</section>

{#if showFeedbackList}
  <CompanyFeedbackListDialog
    {slug}
    companyName={company.name}
    onClose={() => (showFeedbackList = false)}
    onWriteFeedback={openFeedbackForm}
  />
{/if}
{#if showFeedbackForm}
  <CompanyFeedbackDialog
    {slug}
    onClose={() => (showFeedbackForm = false)}
    onSaved={(summary) => (feedbackOverride = summary)}
  />
{/if}
