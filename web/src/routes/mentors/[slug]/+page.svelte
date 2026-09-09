<script lang="ts">
  import { page } from '$app/state';
  import MentorBookingView from '$lib/components/MentorBookingView.svelte';
  import Seo from '$lib/components/Seo.svelte';
  import { Badge } from '$lib/ui';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const mentor = $derived(data.mentor);
  const canonical = $derived(`${page.url.origin}/mentors/${mentor.slug}`);
  const description = $derived(
    `${mentor.headline} at ${mentor.company_name}. Book a ${mentor.session_minutes}-minute conversation.`,
  );
</script>

<Seo title={`${mentor.name} · Mentors · freehire`} {description} {canonical} />

<div class="mx-auto flex w-full max-w-4xl flex-col gap-6 px-4 py-6">
  <header class="flex flex-col gap-2">
    <div class="flex items-center gap-4">
      {#if mentor.show_photo}
        <img
          src={`/api/v1/mentors/${mentor.slug}/photo`}
          alt=""
          class="border-border size-16 shrink-0 rounded-full border object-cover"
          onerror={(e) => {
            (e.currentTarget as HTMLImageElement).style.display = 'none';
          }}
        />
      {/if}
      <h1 class="text-2xl font-semibold">{mentor.name}</h1>
    </div>
    <p class="text-muted-foreground">{mentor.headline} · {mentor.company_name}</p>

    {#if mentor.rating_count > 0}
      <p class="text-sm">{mentor.rating_avg.toFixed(1)} ★ from {mentor.rating_count} sessions</p>
    {/if}

    {#if mentor.topics.length > 0 || mentor.languages.length > 0}
      <div class="flex flex-wrap gap-1">
        {#each mentor.topics as topic (topic)}
          <Badge variant="secondary">{topic}</Badge>
        {/each}
        {#each mentor.languages as language (language)}
          <Badge variant="outline">{language}</Badge>
        {/each}
      </div>
    {/if}
  </header>

  {#if mentor.bio}
    <p class="whitespace-pre-line">{mentor.bio}</p>
  {/if}

  <MentorBookingView {mentor} />
</div>
