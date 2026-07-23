<script lang="ts">
  // The live centre preview: renders a CV Document as ATS-style HTML paginated into discrete A4
  // sheets (page 1, page 2, …), mirroring how Typst paginates the PDF. It measures each top-level
  // block (header, one per experience/project entry, each short section) in a hidden layer, then
  // greedily packs blocks onto A4 page bodies via paginateBlocks so a section never straddles the
  // inter-page gap. Page geometry — content width, sheet padding, and the page body height used for
  // pagination — is derived from the document's per-side margins, so preview and PDF agree. The look
  // tracks the selected template (classic / centered / modern-sans / sidebar). `zoom` scales the
  // whole stack. String composition lives in $lib/cv (unit-tested); this file is layout only.
  import type { Document, ExperienceItem, Project } from '$lib/generated/contracts';
  import { experienceHeader, educationLine, languageLabel, certificationLine } from '$lib/cv';
  import { paginateBlocks } from './geometry';

  let { doc, templateId = 'classic-ats', zoom = 1 }: { doc: Document; templateId?: string; zoom?: number } = $props();

  // A4 at 96dpi, and the inch→pixel factor margins convert through.
  const PAGE_W = 794;
  const PAGE_H = 1123;
  const PX_PER_IN = 96;
  const COL_GAP = 24; // matches the sidebar grid's gap-6

  // Per-template presentation flags (default to classic for any unknown id).
  const isCentered = $derived(templateId === 'centered');
  const isSans = $derived(templateId === 'modern-sans');
  const isSidebar = $derived(templateId === 'sidebar');
  const ruled = $derived(isCentered || isSans || isSidebar); // a rule under each section heading
  const contactSep = $derived(isSans ? '·' : '|');

  // Page geometry from the document's margins (inches → px). A missing or zero side falls back to
  // 0.5in, matching the backend clampMargin / Typst mg rule so preview and PDF never diverge.
  const mt = $derived((doc.margins?.top || 0.5) * PX_PER_IN);
  const mr = $derived((doc.margins?.right || 0.5) * PX_PER_IN);
  const mb = $derived((doc.margins?.bottom || 0.5) * PX_PER_IN);
  const ml = $derived((doc.margins?.left || 0.5) * PX_PER_IN);
  const contentWidth = $derived(PAGE_W - ml - mr);
  const pageBodyHeight = $derived(PAGE_H - mt - mb);
  // Width the paginating (main) column renders at — full content width, or the sidebar's wide column.
  const mainWidth = $derived(isSidebar ? Math.round(contentWidth * 0.65 - COL_GAP) : contentWidth);

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

  // A Block is the atomic unit pagination distributes across sheets. Experience/projects split per
  // entry (the section heading rides with the first entry); short sections are one block each.
  type Block =
    | { id: string; kind: 'header' }
    | { id: string; kind: 'exp'; item: ExperienceItem; heading: boolean }
    | { id: string; kind: 'proj'; item: Project; heading: boolean }
    | { id: string; kind: 'education' }
    | { id: string; kind: 'list'; title: string; items: string[]; sep: string };

  const blocks = $derived.by<Block[]>(() => {
    const bl: Block[] = [{ id: 'header', kind: 'header' }];
    experience.forEach((e, i) => bl.push({ id: `exp-${i}`, kind: 'exp', item: e, heading: i === 0 }));
    projects.forEach((p, i) => bl.push({ id: `proj-${i}`, kind: 'proj', item: p, heading: i === 0 }));
    if (education.length) bl.push({ id: 'education', kind: 'education' });
    // In the sidebar layout skills/languages/certs live in the narrow column, not the main flow.
    if (!isSidebar) {
      if (skills.length) bl.push({ id: 'skills', kind: 'list', title: 'Skills', items: skills, sep: ', ' });
      if (languages.length) bl.push({ id: 'languages', kind: 'list', title: 'Languages', items: languages, sep: ', ' });
      if (certifications.length) bl.push({ id: 'certs', kind: 'list', title: 'Certifications', items: certifications, sep: '; ' });
    }
    return bl;
  });

  // Measured pixel height of each block, kept in sync with a ResizeObserver over the hidden layer.
  // The effect writes heights but never reads them, so it can't re-trigger itself.
  let measureRefs = $state<(HTMLElement | null)[]>([]);
  let heights = $state<number[]>([]);
  $effect(() => {
    const els = measureRefs.slice(0, blocks.length);
    if (els.length !== blocks.length || els.some((el) => !el)) return;
    const present = els as HTMLElement[];
    // Each block's flow advance = the gap to the next block's top (so collapsed inter-block margins
    // count), and the last block's own height. offsetHeight alone would drop those margins and let a
    // page pack slightly past its body.
    const measure = () => {
      const tops = present.map((el) => el.offsetTop);
      heights = present.map((el, i) => (i + 1 < present.length ? tops[i + 1]! - tops[i]! : el.offsetHeight));
    };
    const ro = new ResizeObserver(measure);
    present.forEach((el) => ro.observe(el));
    measure();
    return () => ro.disconnect();
  });

  // Before measurement completes, fall back to zero heights (everything on one sheet), then
  // re-paginate once real heights land.
  const measured = $derived(heights.length === blocks.length ? heights : blocks.map(() => 0));
  const pages = $derived(paginateBlocks(measured, pageBodyHeight));
  // Resolve each page's block indices to the blocks themselves for type-safe iteration.
  const pageBlocks = $derived<Block[][]>(pages.map((pg) => pg.map((i) => blocks[i]).filter((b): b is Block => !!b)));
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

{#snippet headerBlock()}
  <header class={['mb-1', isCentered ? 'text-center' : '']}>
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
{/snippet}

{#snippet experienceItem(e: ExperienceItem)}
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
{/snippet}

{#snippet projectItem(p: Project)}
  {@const bullets = (p.bullets ?? []).filter((b) => b.trim())}
  <ul class="ml-4 list-disc">
    <li class="mb-0.5">
      <span class="font-bold">{p.name}</span>{#if bullets.length}: {bullets.join(' ')}{/if}
      {#if (p.link ?? '').trim()}
        <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external URL from the user's CV, not an internal route -->
        (<a href={p.link} target="_blank" rel="noopener" class="text-[#2b6cb0] hover:underline">{p.link}</a>)
      {/if}
    </li>
  </ul>
{/snippet}

{#snippet listBlock(title: string, items: string[], sep: string)}
  <section class="mb-3">
    {@render sectionHeading(title)}
    <p>{items.join(sep)}</p>
  </section>
{/snippet}

{#snippet blockView(b: Block)}
  {#if b.kind === 'header'}
    {@render headerBlock()}
  {:else if b.kind === 'exp'}
    {#if b.heading}{@render sectionHeading('Experience')}{/if}
    {@render experienceItem(b.item)}
  {:else if b.kind === 'proj'}
    {#if b.heading}{@render sectionHeading('Projects')}{/if}
    {@render projectItem(b.item)}
  {:else if b.kind === 'education'}
    <section class="mb-3">
      {@render sectionHeading('Education')}
      {#each education as line, i (i)}<p class="mb-0.5">{line}</p>{/each}
    </section>
  {:else if b.kind === 'list'}
    {@render listBlock(b.title, b.items, b.sep)}
  {/if}
{/snippet}

{#snippet sidebarColumn()}
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
  {#if skills.length}{@render listBlock('Skills', skills, ', ')}{/if}
  {#if languages.length}{@render listBlock('Languages', languages, ', ')}{/if}
  {#if certifications.length}{@render listBlock('Certifications', certifications, '; ')}{/if}
{/snippet}

<!-- Hidden measurement layer: renders every block once at the main column width so the effect can
     read each block's height. Off-screen and aria-hidden so it never affects layout or a11y. Kept
     OUTSIDE the zoomed stack below — otherwise CSS zoom would scale the measured heights. -->
<div
  aria-hidden="true"
  class={['pointer-events-none invisible absolute -left-[9999px] top-0 text-[13px] leading-snug text-neutral-900', isSans ? 'font-sans' : 'font-serif']}
  style="width: {mainWidth}px;"
>
  {#each blocks as b, i (b.id)}
    <div bind:this={measureRefs[i]}>{@render blockView(b)}</div>
  {/each}
</div>

<!-- Visible A4 sheets, stacked. The CSS `zoom` property scales the whole stack — layout included —
     so the scroll container reserves the scaled size (transform: scale would keep the 794px box and
     clip the left edge on first paint). `items-center` centres each sheet when it fits and lets it
     overflow-scroll when zoomed past the column width. -->
<div class="flex flex-col items-center gap-6" style="zoom: {zoom};">
  {#each pageBlocks as page, p (p)}
    <article
      class={['bg-white text-[13px] leading-snug text-neutral-900 shadow-sm', isSans ? 'font-sans' : 'font-serif']}
      style="width: {PAGE_W}px; min-height: {PAGE_H}px; padding: {mt}px {mr}px {mb}px {ml}px;"
    >
      {#if isSidebar}
        <div class="grid grid-cols-[35%_1fr] gap-6">
          <div>{#if p === 0}{@render sidebarColumn()}{/if}</div>
          <div>
            {#each page as b (b.id)}{@render blockView(b)}{/each}
          </div>
        </div>
      {:else}
        {#each page as b (b.id)}{@render blockView(b)}{/each}
      {/if}
    </article>
  {/each}
</div>
