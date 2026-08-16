<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import Seo from '$lib/components/Seo.svelte';

  const repoUrl = 'https://github.com/strelov1/freehire';
  const telegramUrl = 'https://t.me/freehiredev';
  const contactEmail = 'hello@freehire.me';

  const canonical = $derived(`${page.url.origin}/terms`);

  // Static effective date — bump whenever the terms change.
  const lastUpdated = 'August 10, 2026';
</script>

<Seo
  title="Terms of Service — freehire"
  description="The terms for using freehire, the open-source IT job aggregator: your account, acceptable use, job listings, AI features, and liability."
  {canonical}
/>

<div class="mx-auto w-full max-w-3xl px-4 py-6">
  <div class="flex flex-col gap-8">
    <header class="flex flex-col gap-3">
      <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// legal</p>
      <h1 class="text-4xl font-semibold leading-[1.0] tracking-tighter sm:text-5xl">
        Terms of Service
      </h1>
      <p class="text-sm text-muted-foreground">Last updated: {lastUpdated}</p>
    </header>

    <p class="text-base leading-relaxed text-muted-foreground">
      freehire (<a
        href="https://freehire.me"
        class="font-medium text-foreground underline-offset-4 hover:underline">freehire.me</a
      >), including our mobile app, is a free, open-source IT job aggregator. By using it, you
      agree to these terms.
    </p>

    <section class="flex flex-col gap-3">
      <h2 class="text-xl font-semibold tracking-tight">The service</h2>
      <p class="text-sm leading-relaxed text-muted-foreground">
        We aggregate job postings from public company career boards and other public sources,
        normalise and deduplicate them, and let you search, filter, save, and track them. We also
        offer optional AI features — CV matching and match analysis — that you choose to run.
      </p>
    </section>

    <section class="flex flex-col gap-3">
      <h2 class="text-xl font-semibold tracking-tight">Your account</h2>
      <p class="text-sm leading-relaxed text-muted-foreground">
        You're responsible for the activity on your account and for keeping your credentials
        secure. You can delete your account at any time from account settings, or from the app —
        see <a
          href={resolve('/delete-account')}
          class="font-medium text-foreground underline-offset-4 hover:underline">Account Deletion</a
        > for how.
      </p>
    </section>

    <section class="flex flex-col gap-3">
      <h2 class="text-xl font-semibold tracking-tight">Acceptable use</h2>
      <p class="text-sm leading-relaxed text-muted-foreground">
        Don't misuse the service: no scraping beyond our documented API, no attempting to bypass
        rate limits or authentication, no uploading unlawful content, and no using the service to
        harass or defraud anyone. We may suspend or terminate accounts that violate this.
      </p>
    </section>

    <section class="flex flex-col gap-3">
      <h2 class="text-xl font-semibold tracking-tight">Job listings</h2>
      <p class="text-sm leading-relaxed text-muted-foreground">
        Job postings are aggregated from third-party sources. We don't control their content or
        accuracy, we don't guarantee a listing is still open, and applying is between you and the
        employer — we're not a party to that relationship.
      </p>
    </section>

    <section class="flex flex-col gap-3">
      <h2 class="text-xl font-semibold tracking-tight">AI features</h2>
      <p class="text-sm leading-relaxed text-muted-foreground">
        Features like match analysis and CV matching use a language model and can be wrong. They're
        a research aid, not career or legal advice — use your own judgment before acting on them.
      </p>
    </section>

    <section class="flex flex-col gap-3">
      <h2 class="text-xl font-semibold tracking-tight">No warranty, limited liability</h2>
      <p class="text-sm leading-relaxed text-muted-foreground">
        The service is provided "as is," without warranties of any kind. To the extent permitted
        by law, we're not liable for indirect, incidental, or consequential damages arising from
        your use of it.
      </p>
    </section>

    <section class="flex flex-col gap-3">
      <h2 class="text-xl font-semibold tracking-tight">Changes</h2>
      <p class="text-sm leading-relaxed text-muted-foreground">
        We may update these terms as the product changes. Material changes will be reflected here
        with a new "last updated" date.
      </p>
    </section>

    <section class="flex flex-col gap-3">
      <h2 class="text-xl font-semibold tracking-tight">Contact</h2>
      <p class="text-sm leading-relaxed text-muted-foreground">
        Questions about these terms? Reach us at
        <a
          href="mailto:{contactEmail}"
          class="font-medium text-foreground underline-offset-4 hover:underline">{contactEmail}</a
        >, on
        <a href={telegramUrl} class="font-medium text-foreground underline-offset-4 hover:underline"
          >Telegram</a
        >, or via
        <a href={repoUrl} class="font-medium text-foreground underline-offset-4 hover:underline"
          >GitHub</a
        >.
      </p>
    </section>
  </div>
</div>
