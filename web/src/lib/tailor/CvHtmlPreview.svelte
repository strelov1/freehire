<script lang="ts">
  // The live centre preview: renders a CV Document as an ATS-style HTML resume whose look tracks
  // the selected template (classic / centered / modern-sans / sidebar), mirroring the Typst
  // templates' identity — font, header alignment, section-heading style, and single- vs
  // two-column layout. It is a pure function of `doc` + `templateId`, so it re-renders instantly
  // as the editor mutates the shared document — no network, no PDF. String composition lives in
  // $lib/cv (unit-tested); this file is layout only. `zoom` scales the fixed-width A4 page.
  import type { Document } from '$lib/generated/contracts';
  import { experienceHeader, educationLine, languageLabel, certificationLine } from '$lib/cv';

  let { doc, templateId = 'classic-ats', zoom = 1 }: { doc: Document; templateId?: string; zoom?: number } = $props();

  // Per-template presentation flags (default to classic for any unknown id).
  const isCentered = $derived(templateId === 'centered');
  const isSans = $derived(templateId === 'modern-sans');
  const isSidebar = $derived(templateId === 'sidebar');
  const ruled = $derived(isCentered || isSans || isSidebar); // a rule under each section heading
  const contactSep = $derived(isSans ? '·' : '|');

  const header = $derived(doc.header ?? {});
  const contacts = $derived(
    [header.phone, header.email, header.location, ...(header.links ?? [])]
      .map((c) => (c ?? '').trim())
      .filter((c) => c !== ''),
  );
  const experience = $derived((doc.experience ?? []).filter((e) => experienceHeader(e) !== '' || (e.bullets ?? []).length > 0));
  const projects = $derived((doc.projects ?? []).filter((p) => (p.name ?? '').trim() !== ''));
  const education = $derived((doc.education ?? []).map(educationLine).filter((l) => l !== ''));
  const skills = $derived((doc.skills ?? []).flatMap((g) => g.items ?? []).map((s) => s.trim()).filter((s) => s !== ''));
  const languages = $derived((doc.languages ?? []).map(languageLabel).filter((l) => l !== ''));
  const certifications = $derived((doc.certifications ?? []).map(certificationLine).filter((l) => l !== ''));
  const isLink = (c: string) => /^https?:\/\//i.test(c);
</script>

{#snippet sectionHeading(title: string)}
  <h2 class={['mb-1 mt-3 text-[12px] font-bold uppercase tracking-wide', isCentered ? 'text-center' : '']}>{title}</h2>
  {#if ruled}<hr class="mb-2 -mt-0.5 border-neutral-300" />{/if}
{/snippet}

{#snippet contactLine()}
  <p class={['text-[12px] text-neutral-700', isCentered ? 'text-center' : '', isSans ? 'text-neutral-500' : '']}>
    {#each contacts as c, i (i)}
      {#if i > 0}<span class="mx-1.5 text-neutral-400">{contactSep}</span>{/if}
      {#if isLink(c)}
        <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external URL from the user's CV, not an internal route -->
        <a href={c} target="_blank" rel="noopener" class="text-[#2b6cb0] hover:underline">{c}</a>
      {:else}{c}{/if}
    {/each}
  </p>
{/snippet}

{#snippet experienceBlock()}
  {#if experience.length}
    <section class="mb-3">
      {@render sectionHeading('Experience')}
      {#each experience as e, i (i)}
        {@const bullets = (e.bullets ?? []).filter((b) => b.trim())}
        {@const stack = (e.stack ?? []).filter((s) => s.trim())}
        <div class="mb-2.5">
          <p class="font-bold">{experienceHeader(e)}</p>
          {#if (e.summary ?? '').trim()}<p class="text-neutral-800">{e.summary}</p>{/if}
          {#if bullets.length}
            <ul class="ml-4 list-disc space-y-0.5">
              {#each bullets as b, bi (bi)}<li>{b}</li>{/each}
            </ul>
          {/if}
          {#if stack.length}
            <p class="mt-0.5"><span class="font-bold">Stack:</span> {stack.join(', ')}</p>
          {/if}
        </div>
      {/each}
    </section>
  {/if}
{/snippet}

{#snippet projectsBlock()}
  {#if projects.length}
    <section class="mb-3">
      {@render sectionHeading('Projects')}
      <ul class="ml-4 list-disc space-y-0.5">
        {#each projects as p, i (i)}
          {@const bullets = (p.bullets ?? []).filter((b) => b.trim())}
          <li>
            <span class="font-bold">{p.name}</span>{#if bullets.length}: {bullets.join(' ')}{/if}
            {#if (p.link ?? '').trim()}
              <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external URL from the user's CV, not an internal route -->
              (<a href={p.link} target="_blank" rel="noopener" class="text-[#2b6cb0] hover:underline">{p.link}</a>)
            {/if}
          </li>
        {/each}
      </ul>
    </section>
  {/if}
{/snippet}

{#snippet educationBlock()}
  {#if education.length}
    <section class="mb-3">
      {@render sectionHeading('Education')}
      {#each education as line, i (i)}<p class="mb-0.5">{line}</p>{/each}
    </section>
  {/if}
{/snippet}

{#snippet listBlock(title: string, items: string[], sep: string)}
  {#if items.length}
    <section class="mb-3">
      {@render sectionHeading(title)}
      <p>{items.join(sep)}</p>
    </section>
  {/if}
{/snippet}

<!-- A4 page (794px ≈ 210mm @96dpi). The CSS `zoom` property scales the whole page box — layout
     included — so the scroll container reserves the scaled size. (transform: scale keeps the
     794px box, so with justify-center the left edge overflows unreachably and clips on first
     paint.) `mx-auto` centres the page when it fits and left-aligns it when it overflows, so it
     never clips. The serif/sans + black-on-white styling reads like the printed PDF; the look
     tracks the selected template. -->
<article
  class={[
    'mx-auto w-[794px] bg-white px-14 py-12 text-[13px] leading-snug text-neutral-900 shadow-sm',
    isSans ? 'font-sans' : 'font-serif',
  ]}
  style="zoom: {zoom};"
>
  <!-- Header -->
  <header class={['mb-3', isCentered ? 'text-center' : '']}>
    <h1 class={['text-2xl font-bold', isSans ? 'uppercase tracking-wider' : 'tracking-tight']}>
      {header.full_name || 'Your Name'}
    </h1>
    {#if !isSidebar && contacts.length}
      <div class="mt-1">{@render contactLine()}</div>
    {/if}
    {#if (doc.summary ?? '').trim()}
      <p
        class={[
          'mt-2 text-[12.5px] text-neutral-800',
          isCentered ? 'mx-auto max-w-[62ch] italic' : '',
          isSidebar ? 'italic' : '',
        ]}
      >
        {doc.summary}
      </p>
    {/if}
  </header>

  <hr class={['my-2', isSans || isSidebar ? 'border-neutral-500' : 'border-neutral-300']} />

  {#if isSidebar}
    <!-- Two-column body: narrow left (contact/links/skills/languages), wide right (experience/education/projects). -->
    <div class="grid grid-cols-[35%_1fr] gap-6">
      <div>
        {#if contacts.length}
          <section class="mb-3">
            {@render sectionHeading('Contact')}
            {#each contacts as c, i (i)}
              {#if isLink(c)}
                <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external URL from the user's CV, not an internal route -->
                <p class="mb-0.5 break-words"><a href={c} target="_blank" rel="noopener" class="text-[#2b6cb0] hover:underline">{c}</a></p>
              {:else}<p class="mb-0.5 break-words">{c}</p>{/if}
            {/each}
          </section>
        {/if}
        {@render listBlock('Skills', skills, ', ')}
        {@render listBlock('Languages', languages, ', ')}
        {@render listBlock('Certifications', certifications, '; ')}
      </div>
      <div>
        {@render experienceBlock()}
        {@render educationBlock()}
        {@render projectsBlock()}
      </div>
    </div>
  {:else}
    {@render experienceBlock()}
    {@render projectsBlock()}
    {@render educationBlock()}
    {@render listBlock('Skills', skills, ', ')}
    {@render listBlock('Languages', languages, ', ')}
    {@render listBlock('Certifications', certifications, '; ')}
  {/if}
</article>
