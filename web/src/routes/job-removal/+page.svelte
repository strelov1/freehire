<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import Seo from '$lib/components/Seo.svelte';

  const contactEmail = 'hello@freehire.me';
  const canonical = $derived(`${page.url.origin}/job-removal`);
  const link = 'font-medium text-foreground underline-offset-4 hover:underline';
</script>

<Seo
  title="Job Posting Removal Requests — freehire"
  description="Why freehire does not delete job postings on request, how a posting leaves the site on its own, and how to ask for a correction."
  {canonical}
/>

<div class="mx-auto w-full max-w-3xl px-4 py-6">
  <div class="flex flex-col gap-8">
    <header class="flex flex-col gap-3">
      <p class="font-mono text-xs uppercase tracking-widest text-muted-foreground">// legal</p>
      <h1 class="text-4xl font-semibold leading-none tracking-tighter sm:text-5xl">
        Job Posting Removal Requests
      </h1>
    </header>

    <section class="flex flex-col gap-3">
      <h2 class="text-xl font-semibold tracking-tight">How your posting got here</h2>
      <p class="text-sm leading-relaxed text-muted-foreground">
        freehire shows publicly available job postings. Many of them are added by our users, who
        save the vacancies they find, track their applications, and share them with each other.
        Every posting links back to its original source, where candidates apply.
      </p>
    </section>

    <section class="flex flex-col gap-3">
      <h2 class="text-xl font-semibold tracking-tight">Why we don't delete postings</h2>
      <p class="text-sm leading-relaxed text-muted-foreground">
        One of freehire's goals is to give job seekers honest information about a vacancy,
        including its history: when it first appeared, how long it stayed open, and whether it was
        reposted. That history is how we flag
        <a href={resolve('/features/ghost-jobs')} class={link}>ghost jobs</a> — roles that stay
        listed with no real intent to hire. Deleting postings on request would erase exactly the
        record candidates rely on, so we keep them.
      </p>
    </section>

    <section class="flex flex-col gap-3">
      <h2 class="text-xl font-semibold tracking-tight">Postings close on their own</h2>
      <p class="text-sm leading-relaxed text-muted-foreground">
        freehire updates postings automatically. When a vacancy is taken down at its source, we
        detect it and mark it as closed here too. If you no longer want your posting to appear as
        open, unpublish it at the source — it will drop out of freehire's listings within a short
        time, with nothing else for you to do.
      </p>
    </section>

    <section class="flex flex-col gap-3">
      <h2 class="text-xl font-semibold tracking-tight">Asking for a correction</h2>
      <p class="text-sm leading-relaxed text-muted-foreground">
        If a posting shows information you don't want displayed — personal contact details, for
        example — email us at
        <a href="mailto:{contactEmail}" class={link}>{contactEmail}</a>
        with a link to the posting and what should change. We review every request and edit the
        posting where it's warranted.
      </p>
    </section>
  </div>
</div>
