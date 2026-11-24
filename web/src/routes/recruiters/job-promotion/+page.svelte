<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { ArrowLeft, Mail } from '@lucide/svelte';
  import Seo from '#lib/components/Seo.svelte';
  import { mailtoHref } from '#lib/followup.js';
  import { Button, Input } from '#lib/ui/index.js';

  const path = resolve('/recruiters/job-promotion');
  const recipient = 'hello@freehire.me';
  const subject = 'Job promotion pilot';
  const contactHref = `mailto:${recipient}?subject=${encodeURIComponent(subject)}`;

  let email = $state('');
  let vacancyUrl = $state('');

  function openDraft(event: SubmitEvent) {
    event.preventDefault();
    location.assign(
      mailtoHref({
        recipient,
        subject,
        body: `Work email: ${email.trim()}\r\nVacancy URL: ${vacancyUrl.trim()}`,
      }),
    );
  }
</script>

<Seo
  title="Job promotion pilot for tech vacancies — freehire"
  description="Tell freehire which tech vacancy you want more candidates to see. Apply for the job promotion pilot with a work email and a job link."
  canonical={page.url.origin + path}
/>

<div class="mx-auto w-full max-w-6xl px-4 pb-16 pt-6 sm:pb-24">
  <a
    href={resolve('/recruiters')}
    class="inline-flex min-h-11 items-center gap-2 text-sm font-medium underline-offset-4 hover:underline focus-visible:underline"
  >
    <ArrowLeft class="size-4" aria-hidden="true" />
    For recruiters
  </a>

  <div class="grid items-start gap-y-8 pt-8 lg:grid-cols-2 lg:gap-x-16 lg:pt-14">
    <div class="flex flex-col gap-6">
      <h1 class="text-balance text-4xl font-semibold leading-none tracking-tight sm:text-6xl">
        Put a tech vacancy in front of more candidates.
      </h1>
      <p class="max-w-xl text-lg leading-relaxed text-muted-foreground">
        We’re exploring ways to give individual roles more visibility on freehire. Share the vacancy you want to promote
        and we’ll follow up by email about the pilot.
      </p>
    </div>

    <section class="min-w-0 rounded-xl bg-muted p-6 sm:p-8 lg:row-span-2" aria-labelledby="pilot-request-title">
      <h2 id="pilot-request-title" class="text-2xl font-semibold tracking-tight">Apply for the promotion pilot</h2>
      <p id="pilot-email-hint" class="mt-2 text-sm leading-relaxed text-muted-foreground">
        The button opens a draft in your email app. Review it and send it from there.
      </p>
      <form
        class="mt-6 flex flex-col items-start gap-5"
        action={contactHref}
        method="post"
        enctype="text/plain"
        onsubmit={openDraft}
        aria-describedby="pilot-email-hint"
      >
        <div class="flex w-full flex-col gap-2">
          <label for="pilot-work-email" class="text-sm font-medium">Work email</label>
          <Input
            id="pilot-work-email"
            name="Work email"
            type="email"
            autocomplete="email"
            placeholder="you@company.com"
            bind:value={email}
            required
            class="w-full bg-background"
          />
        </div>
        <div class="flex w-full flex-col gap-2">
          <label for="pilot-vacancy-url" class="text-sm font-medium">Vacancy URL</label>
          <Input
            id="pilot-vacancy-url"
            name="Vacancy URL"
            type="url"
            inputmode="url"
            pattern="[Hh][Tt][Tt][Pp][Ss]?://.+"
            placeholder="https://company.com/careers/role"
            bind:value={vacancyUrl}
            required
            aria-describedby="pilot-url-hint"
            class="w-full bg-background"
          />
          <p id="pilot-url-hint" class="text-xs text-muted-foreground">
            Use a public link starting with https:// or http://.
          </p>
        </div>
        <Button type="submit" variant="primary" size="lg"
          >Open email draft <Mail class="size-4" aria-hidden="true" /></Button
        >
      </form>
      <p class="mt-5 border-t border-border pt-4 text-sm leading-relaxed text-muted-foreground">
        You can also send the vacancy link from your work email to
        <a href={contactHref} class="font-medium text-foreground underline underline-offset-4">{recipient}</a>.
      </p>
    </section>

    <dl class="flex flex-col divide-y divide-border border-y border-border">
      <div class="py-4">
        <dt class="font-semibold">Which vacancies can I share?</dt>
        <dd class="mt-1 text-sm leading-relaxed text-muted-foreground">
          Send a public link to an open tech role on freehire, your careers site, or your applicant tracking system.
        </dd>
      </div>
      <div class="py-4">
        <dt class="font-semibold">What happens after I email?</dt>
        <dd class="mt-1 text-sm leading-relaxed text-muted-foreground">
          We review the role and get in touch about pilot availability and terms.
        </dd>
      </div>
    </dl>
  </div>
</div>
