<script lang="ts">
  import { page } from '$app/state';
  import CandidateCardView from '#lib/components/CandidateCardView.svelte';
  import Seo from '#lib/components/Seo.svelte';
  import { breadcrumbJsonLd, jsonLdScript } from '#lib/seo.js';
  import { talentHeading, talentPlace } from '#lib/talentCard.js';
  import { Breadcrumbs } from '#lib/ui/index.js';
  import type { PageData } from './$types';

  // One member's public card. The same anonymised payload the catalogue list carries,
  // with the work history opened up (CandidateCardView, shared with the interview-buddy
  // member page). No name, no employer, no prose — the payload has none.

  let { data }: { data: PageData } = $props();

  const member = $derived(data.member);
  const card = $derived(member.card);

  const heading = $derived(talentHeading(card.seniority, card.category, 'Candidate'));
  const { country, zone, place } = $derived(talentPlace(member));

  const origin = $derived(page.url.origin);
  const canonical = $derived(`${origin}/talent/${member.handle}`);
  // The one breadcrumb trail feeding both the visible nav and the structured data below —
  // a single array so the two can never disagree about what the trail is.
  const breadcrumbItems = $derived([{ name: 'Talent Network', href: '/talent' }, { name: heading }]);
  const jsonLd = $derived(
    jsonLdScript([
      breadcrumbJsonLd(
        breadcrumbItems.map((item) => ({
          name: item.name,
          url: item.href ? `${origin}${item.href}` : canonical,
        })),
      ),
    ]),
  );
</script>

<Seo
  title="{heading} — freehire Talent Network"
  description="An anonymous candidate profile from freehire's Talent Network."
/>
<svelte:head>
  <!-- A card is a person, and a search engine's cache of one would outlive their decision
  to leave the network. The list page is indexable; an individual card is not. -->
  <meta name="robots" content="noindex" />
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-5 py-6 sm:px-4">
  <Breadcrumbs items={breadcrumbItems} class="mb-4" />

  <CandidateCardView
    {card}
    photoSrc="/api/v1/talent/{member.handle}/photo"
    {place}
    {zone}
    {country}
    specializations={member.specializations}
  />
</div>
