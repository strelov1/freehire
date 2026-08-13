<script lang="ts">
  import { resolve } from '$app/paths';
  import { Button } from '$lib/ui';
  import { NumberedGrid, SectionLabel } from '$lib/ui';
  import { TAILOR_FAQ } from '$lib/tailorFaq';

  // The requirement split the tailoring context hands the agent. This is the
  // whole honesty argument of the feature, so it leads the page: what your
  // history covers is reframed, what it doesn't is asked about, never written.
  const split = [
    {
      key: 'have',
      title: 'Already yours, buried',
      body: 'Requirements your history covers but the CV states in passing, or three roles down. These get pulled forward and said plainly.',
      sample: ['Kafka at scale', 'Payments migrations', 'Mentoring'],
    },
    {
      key: 'gap',
      title: 'Not yours yet',
      body: "Requirements nothing in your history supports. The agent asks you before anything is written — it cannot fill these in on its own.",
      sample: ['Kubernetes operators', 'Rust'],
    },
  ];

  // The parts of the CV an edit can address (internal/cvedit). Listed so the page's "you can
  // see every edit" claim is concrete rather than a mood — and every one of them is recorded
  // in the CV's history, where it can be undone on its own.
  const ops = [
    'summary',
    'experience[i].bullets[j]',
    'experience[i].stack[j]',
    'skills[i].items[j]',
    'projects[i].bullets[j]',
    'education[i].degree',
    'certifications[i].issuer',
    'style.font_size',
  ];

  const steps = [
    {
      n: '01',
      title: 'Analyse the fit',
      body: 'Run the AI fit analysis on the vacancy. It scores the match dimension by dimension and names what is missing — the reasoning the tailored CV will work from.',
    },
    {
      n: '02',
      title: 'Tailor from the result',
      body: 'One click copies your base CV into a new one bound to that vacancy. Your original is untouched; each job gets its own copy, so nothing you tailor bleeds into the next application.',
    },
    {
      n: '03',
      title: 'Edit with the agent, or by hand',
      body: 'The agent reads the analysis and your CV before it says anything, then proposes edits one at a time. The editor is right there — you can overrule any of it and type your own.',
    },
    {
      n: '04',
      title: 'Export',
      body: 'Pick a template and download the PDF. Switching templates never touches the content, so you can see the same CV three ways before you send it.',
    },
  ];
</script>

<div class="flex flex-col">
  <!-- Hero. Left: the pitch. Right: the workspace, as three columns. -->
  <section class="dot-grid -mx-4 grid items-center gap-12 px-4 pb-16 pt-8 lg:grid-cols-[1.05fr_0.95fr]">
    <div>
      <SectionLabel text="cv tailoring" />
      <h1 class="mt-6 max-w-2xl text-balance text-4xl font-semibold leading-[1.0] tracking-tighter sm:text-6xl">
        Rewrite your CV for one job. Invent nothing.
      </h1>
      <p class="mt-7 max-w-xl text-lg leading-relaxed text-muted-foreground">
        A generic CV loses to a specific one, and a fabricated one loses the interview. freehire
        reframes what you have actually done toward the vacancy in front of you — pulling the
        relevant work forward, saying it in the job's own terms, and asking you about anything it
        cannot find in your history.
      </p>
      <div class="mt-9 flex flex-wrap items-center gap-3">
        <Button href={resolve('/')} variant="primary" size="lg">Find a job to tailor for</Button>
        <Button href={resolve('/my/cvs')} variant="outline" size="lg">Your CVs</Button>
      </div>
    </div>

    <!-- Workspace preview: the real three-column layout, in miniature. -->
    <figure class="overflow-hidden rounded-xl border border-border bg-card shadow-sm">
      <figcaption class="flex items-center gap-2 border-b border-border px-4 py-2.5 text-xs text-muted-foreground">
        <span class="size-2.5 rounded-full bg-muted-foreground/30"></span>
        freehire · Tailoring
      </figcaption>
      <div class="grid gap-px bg-border sm:grid-cols-[1fr_1.1fr_0.85fr]">
        <div class="bg-background p-3">
          <div class="flex gap-2 text-[11px]">
            <span class="rounded-full bg-secondary px-2 py-0.5 font-medium">Chat</span>
            <span class="text-muted-foreground">Editor</span>
          </div>
          <div class="mt-3 flex flex-col gap-2">
            <p class="rounded-lg border border-border px-2.5 py-2 text-[11px] leading-relaxed text-muted-foreground">
              Your Acme role already covers the event-bus requirement — want me to lead with it?
            </p>
            <p class="ml-4 rounded-lg bg-secondary px-2.5 py-2 text-[11px] leading-relaxed">Yes, and drop the CMS line.</p>
          </div>
        </div>
        <div class="bg-background p-3">
          <p class="text-[11px] text-muted-foreground">Preview</p>
          <div class="mt-3 space-y-1.5">
            <div class="h-2 w-2/3 rounded bg-foreground/70"></div>
            <div class="h-1.5 w-1/3 rounded bg-muted-foreground/30"></div>
            <div class="mt-3 h-1.5 w-full rounded bg-muted-foreground/25"></div>
            <div class="h-1.5 w-11/12 rounded bg-muted-foreground/25"></div>
            <div class="h-1.5 w-4/5 rounded bg-brand/40"></div>
            <div class="h-1.5 w-full rounded bg-muted-foreground/25"></div>
            <div class="h-1.5 w-3/4 rounded bg-muted-foreground/25"></div>
          </div>
        </div>
        <div class="bg-background p-3">
          <div class="flex gap-2 text-[11px]">
            <span class="rounded-full bg-secondary px-2 py-0.5 font-medium">Verdict</span>
            <span class="text-muted-foreground">Job</span>
          </div>
          <div class="mt-3">
            <p class="text-2xl font-semibold tabular-nums">72</p>
            <p class="text-[11px] text-muted-foreground">worth applying</p>
            <div class="mt-3 space-y-1.5">
              <div class="h-1.5 w-full rounded bg-emerald-500/40"></div>
              <div class="h-1.5 w-2/3 rounded bg-warning/40"></div>
              <div class="h-1.5 w-1/3 rounded bg-destructive/40"></div>
            </div>
          </div>
        </div>
      </div>
    </figure>
  </section>

  <!-- The split. The argument for why this isn't a lying machine. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="reframe, don't fabricate" />
    <div class="mt-6 max-w-2xl">
      <h2 class="text-3xl font-semibold tracking-tight sm:text-4xl">
        It knows the difference between burying something and not having it.
      </h2>
      <p class="mt-5 leading-relaxed text-muted-foreground">
        Before the agent writes a word, it reads the fit analysis for this vacancy and sorts the
        job's requirements into two piles. That split is the whole feature: one pile is editing,
        the other is a question.
      </p>
    </div>

    <div class="mt-10 grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-2">
      {#each split as s (s.key)}
        <div class="bg-background p-6 sm:p-7">
          <h3 class="text-lg font-semibold tracking-tight">{s.title}</h3>
          <p class="mt-2 text-sm leading-relaxed text-muted-foreground">{s.body}</p>
          <div class="mt-4 flex flex-wrap gap-2">
            {#each s.sample as item (item)}
              <span
                class="rounded-md border px-2 py-0.5 font-mono text-xs {s.key === 'have'
                  ? 'border-emerald-400/50 text-emerald-600 dark:text-emerald-400'
                  : 'border-warning/50 text-warning-strong'}"
              >
                {item}
              </span>
            {/each}
          </div>
        </div>
      {/each}
    </div>

    <p class="mt-6 max-w-3xl text-sm leading-relaxed text-muted-foreground">
      The rule holds below the interface too: an achievement a model inferred is stored marked as
      the model's, and cannot be written into a CV until you confirm it. That check lives in the
      code, not in the agent's instructions — which is the difference between a safeguard and a
      request.
    </p>
  </section>

  <!-- How it works. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="how it works" />
    <NumberedGrid items={steps} class="mt-10 sm:grid-cols-2" />
  </section>

  <!-- Edits you can see. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="every edit, one at a time" />
    <div class="mt-6 grid gap-10 lg:grid-cols-2 lg:items-center">
      <div>
        <h2 class="max-w-md text-3xl font-semibold tracking-tight sm:text-4xl">
          No wholesale rewrite to proofread.
        </h2>
        <p class="mt-5 max-w-md leading-relaxed text-muted-foreground">
          The agent cannot replace your CV in one move. It edits through a deliberately narrow
          vocabulary — one field, one bullet, one ordering at a time — so every change is visible in
          the live preview as it lands, and reversible if you disagree. Type over any of it: the
          editor and the agent write to the same document.
        </p>
        <div class="mt-8 flex flex-wrap gap-3">
          <Button href={resolve('/my/cvs')} variant="primary" size="lg">Open your CVs</Button>
          <Button href={resolve('/cli')} variant="ghost" size="lg">Drive it from the CLI</Button>
        </div>
      </div>

      <div class="flex flex-wrap gap-2">
        {#each ops as op (op)}
          <span class="rounded-md border border-border bg-background/60 px-2.5 py-1 font-mono text-xs text-muted-foreground">
            {op}
          </span>
        {/each}
      </div>
    </div>
  </section>

  <!-- Export. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="export" />
    <div class="mt-6 max-w-2xl">
      <h2 class="text-3xl font-semibold tracking-tight sm:text-4xl">A PDF a parser can read.</h2>
      <p class="mt-5 leading-relaxed text-muted-foreground">
        The CV is typeset into a clean single-column PDF with real text — no tables, no side
        columns, no words baked into an image — so the ATS on the other end reads what you see.
        Templates change the typography, never the content, so you can look at the same CV three
        ways before sending it.
      </p>
    </div>
  </section>

  <!-- Terminal. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="from the terminal" />
    <div class="mt-6 grid gap-10 lg:grid-cols-2 lg:items-center">
      <div>
        <h2 class="max-w-md text-3xl font-semibold tracking-tight sm:text-4xl">
          Or hand the loop to your own agent.
        </h2>
        <p class="mt-5 max-w-md leading-relaxed text-muted-foreground">
          The same flow runs on the freehire CLI with one API key, so a harness you wrote can tailor
          the CV instead: read the analysis, read the document, apply patches, render. Same rules —
          the split is in the context it reads, so your agent inherits the honesty constraint rather
          than being trusted with it.
        </p>
        <div class="mt-8 flex flex-wrap gap-3">
          <Button href={resolve('/cli')} variant="primary" size="lg">Get the CLI</Button>
          <Button href={resolve('/docs/api')} variant="ghost" size="lg">API reference</Button>
        </div>
      </div>

      <figure class="overflow-hidden rounded-xl border border-border bg-secondary/60 font-mono text-sm shadow-sm">
        <figcaption class="flex items-center gap-2 border-b border-border px-4 py-2.5 text-xs text-muted-foreground">
          <span class="size-2.5 rounded-full bg-muted-foreground/30"></span>
          terminal
        </figcaption>
        <pre class="overflow-x-auto p-4 leading-relaxed"><span class="text-muted-foreground"># what to reframe toward, and what to ask about</span>
freehire <span class="text-foreground">cv context &lt;cv-id&gt;</span>

<span class="text-muted-foreground"># the document as JSON</span>
freehire <span class="text-foreground">cv get &lt;cv-id&gt;</span>

<span class="text-muted-foreground"># one field-level edit</span>
freehire <span class="text-foreground">cv edit &lt;cv-id&gt; --op set --path 'experience[0].bullets[1]' --value '…'</span>

<span class="text-muted-foreground"># the PDF</span>
freehire <span class="text-foreground">cv render &lt;cv-id&gt; --out cv.pdf</span></pre>
      </figure>
    </div>
  </section>

  <!-- FAQ. Visible answers and the FAQPage JSON-LD share TAILOR_FAQ. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="faq" />
    <h2 class="mt-6 max-w-md text-3xl font-semibold tracking-tight sm:text-4xl">
      Frequently asked questions.
    </h2>
    <dl class="mt-10 grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-2">
      {#each TAILOR_FAQ as item (item.question)}
        <div class="bg-background p-6 sm:p-7">
          <dt class="text-lg font-semibold tracking-tight">{item.question}</dt>
          <dd class="mt-2 text-sm leading-relaxed text-muted-foreground">{item.answer}</dd>
        </div>
      {/each}
    </dl>
  </section>

  <!-- Closing CTA. -->
  <section class="border-t border-border py-16 sm:py-20">
    <div class="flex flex-col items-start gap-4 rounded-xl border border-border bg-secondary/40 p-6 sm:p-8">
      <h2 class="text-2xl font-semibold tracking-tight">Stop sending the same CV everywhere.</h2>
      <p class="max-w-xl leading-relaxed text-muted-foreground">
        Find a job worth the effort, run the fit analysis, and tailor from it. Ten minutes per
        application, and nothing on the page that you did not do.
      </p>
      <div class="flex flex-wrap gap-3">
        <Button href={resolve('/')} variant="primary" size="lg">Browse jobs</Button>
        <Button href={resolve('/my/cvs')} variant="outline" size="lg">Your CVs</Button>
      </div>
    </div>
  </section>
</div>
