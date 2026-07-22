<script lang="ts">
  // The live centre preview: renders a CV Document as an ATS-style HTML resume that mirrors the
  // classic-ats Typst template (single column, serif, "Company | Location | Role (dates)" role
  // headers, inline Education/Skills/Languages/Certifications). It is a pure function of `doc`, so
  // it re-renders instantly as the editor mutates the shared document — no network, no PDF. The
  // string composition lives in $lib/cv (unit-tested); this file is layout only. `zoom` scales the
  // fixed-width A4 page; the host owns the zoom control and the Download-PDF action.
  import type { Document } from '$lib/generated/contracts';
  import { experienceHeader, educationLine, languageLabel, certificationLine } from '$lib/cv';

  let { doc, zoom = 1 }: { doc: Document; zoom?: number } = $props();

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

<!-- A4 page (794px ≈ 210mm @96dpi), scaled by zoom from the top so the ruler-like preview grows
     downward. The serif + black-on-white styling reads like the printed PDF. -->
<div class="flex justify-center">
  <div style="transform: scale({zoom}); transform-origin: top center;">
    <article
      class="w-[794px] bg-white px-14 py-12 font-serif text-[13px] leading-snug text-neutral-900 shadow-sm"
    >
      <!-- Header -->
      <header class="mb-3 text-center">
        <h1 class="text-2xl font-bold tracking-tight">{header.full_name || 'Your Name'}</h1>
        {#if contacts.length}
          <p class="mt-1 text-[12px] text-neutral-700">
            {#each contacts as c, i (i)}
              {#if i > 0}<span class="mx-1.5 text-neutral-400">|</span>{/if}
              {#if isLink(c)}
                <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external URL from the user's CV, not an internal route -->
                <a href={c} target="_blank" rel="noopener" class="text-[#2b6cb0] hover:underline">{c}</a>
              {:else}{c}{/if}
            {/each}
          </p>
        {/if}
        {#if (doc.summary ?? '').trim()}
          <p class="mx-auto mt-2 max-w-[62ch] text-[12.5px] text-neutral-800">{doc.summary}</p>
        {/if}
      </header>

      <hr class="my-2 border-neutral-300" />

      <!-- Experience -->
      {#if experience.length}
        <section class="mb-3">
          <h2 class="mb-1 text-[12px] font-bold uppercase tracking-wide">Experience</h2>
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

      <!-- Projects -->
      {#if projects.length}
        <section class="mb-3">
          <h2 class="mb-1 text-[12px] font-bold uppercase tracking-wide">Projects</h2>
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

      <!-- Education (inline) -->
      {#if education.length}
        <p class="mb-1.5"><span class="text-[12px] font-bold uppercase tracking-wide">Education</span>&nbsp;&nbsp;{education.join('; ')}</p>
      {/if}

      <!-- Skills (inline) -->
      {#if skills.length}
        <p class="mb-1.5"><span class="font-bold">SKILLS:</span> {skills.join(', ')}</p>
      {/if}

      <!-- Languages (inline) -->
      {#if languages.length}
        <p class="mb-1.5"><span class="font-bold">LANGUAGES:</span> {languages.join(', ')}</p>
      {/if}

      <!-- Certifications (inline) -->
      {#if certifications.length}
        <p class="mb-1.5"><span class="font-bold">CERTIFICATIONS:</span> {certifications.join('; ')}</p>
      {/if}
    </article>
  </div>
</div>
