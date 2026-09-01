<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import Seo from '$lib/components/Seo.svelte';
  import { breadcrumbJsonLd, jsonLdScript } from '$lib/seo';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const origin = $derived(page.url.origin);
  const canonical = $derived(`${origin}/skills`);
  const count = (n: number) => n.toLocaleString('en-US');
  const description = $derived(
    `Plain-language definitions for the ${count(data.total)} ${data.total === 1 ? 'skill' : 'skills'} freehire tags jobs with — what each one is, and who is hiring for it.`,
  );

  const jsonLd = $derived(
    jsonLdScript([
      breadcrumbJsonLd([
        { name: 'freehire', url: `${origin}/` },
        { name: 'Skills', url: canonical },
      ]),
    ]),
  );
</script>

<Seo title="IT Skills Glossary · freehire" {description} {canonical} />
<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD from jsonLdScript, which escapes `<` -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-4xl px-4 py-6">
  <nav class="mb-4 flex items-center gap-2 text-sm text-muted-foreground" aria-label="Breadcrumb">
    <a href={resolve('/')} class="hover:underline">freehire</a>
  </nav>

  <header class="mb-8">
    <h1 class="text-2xl font-semibold tracking-tight">IT Skills Glossary</h1>
    <p class="mt-2 max-w-2xl text-sm leading-relaxed text-muted-foreground">{description}</p>
  </header>

  {#each data.groups as group (group.letter)}
    <section class="mb-6">
      <h2 class="mb-2 text-sm font-semibold tracking-tight text-muted-foreground">
        {group.letter}
      </h2>
      <ul class="flex flex-wrap gap-x-4 gap-y-1">
        {#each group.skills as skill (skill.slug)}
          <li>
            <a href={resolve('/skills/[slug]', { slug: skill.slug })} class="text-sm hover:underline"
              >{skill.label}</a
            >
          </li>
        {/each}
      </ul>
    </section>
  {/each}
</div>
