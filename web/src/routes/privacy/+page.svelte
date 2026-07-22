<script lang="ts">
  import { page } from '$app/state';
  import Seo from '$lib/components/Seo.svelte';

  const repoUrl = 'https://github.com/strelov1/freehire';
  const telegramUrl = 'https://t.me/freehiredev';
  // TODO(strelov1): confirm the contact address you want published here.
  const contactEmail = 'hello@freehire.dev';

  const canonical = $derived(`${page.url.origin}/privacy`);

  // Static effective date — freehire has no Date.now-driven content, and a hard
  // date is what a privacy policy needs. Bump this whenever the policy changes.
  const lastUpdated = 'July 22, 2026';
</script>

<Seo
  title="Privacy Policy — freehire"
  description="How freehire, the open-source IT job aggregator, collects, uses, and stores your data: accounts, job tracking, CV analysis, cookies, and third-party services."
  {canonical}
/>

<div class="mx-auto w-full max-w-3xl px-4 py-6">
  <div class="flex flex-col gap-8">
    <header class="flex flex-col gap-3">
      <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// legal</p>
      <h1 class="text-4xl font-semibold leading-[1.0] tracking-tighter sm:text-5xl">
        Privacy Policy
      </h1>
      <p class="text-sm text-muted-foreground">Last updated: {lastUpdated}</p>
    </header>

    <p class="text-base leading-relaxed text-muted-foreground">
      freehire (<a
        href="https://freehire.dev"
        class="font-medium text-foreground underline-offset-4 hover:underline">freehire.dev</a
      >) is a free, open-source IT job aggregator. This policy explains what data we collect, why,
      and who we share it with. We collect only what the product needs to work, and we never sell
      your data.
    </p>

    <section class="flex flex-col gap-3">
      <h2 class="text-xl font-semibold tracking-tight">What we collect</h2>
      <ul class="flex flex-col gap-3 text-sm leading-relaxed text-muted-foreground">
        <li>
          <span class="font-medium text-foreground">Account data.</span> When you register, we store
          your email address and — for password sign-in — a salted bcrypt hash of your password (never
          the plaintext). If you sign in with Google, GitHub, or LinkedIn, we store your verified
          email and a provider identifier so we can recognise you next time; we do not keep the
          provider's access tokens.
        </li>
        <li>
          <span class="font-medium text-foreground">Job activity.</span> If you save, view, apply to,
          or track a job, we store that interaction (the job, timestamps, application stage, and any
          notes you add) so we can show you your pipeline.
        </li>
        <li>
          <span class="font-medium text-foreground">CV / résumé.</span> If you upload a CV for skill
          matching or AI fit analysis, we store the file and the text we extract from it. Running an
          AI fit analysis sends the relevant parts of your CV, together with the job posting, to our
          language-model provider (see “Third-party services”).
        </li>
        <li>
          <span class="font-medium text-foreground">API keys.</span> If you create a personal API
          key, we store only a SHA-256 hash of it. The key itself is shown once at creation and is
          unrecoverable afterwards.
        </li>
        <li>
          <span class="font-medium text-foreground">Technical data.</span> Standard request logs
          (IP address, user agent, timestamps) and, where you allow it, product-analytics cookies
          (see “Cookies”). We do not sell your data or build advertising profiles.
        </li>
      </ul>
    </section>

    <section class="flex flex-col gap-3">
      <h2 class="text-xl font-semibold tracking-tight">How we use it</h2>
      <p class="text-sm leading-relaxed text-muted-foreground">
        We use your data to run the service: authenticate you, remember your saved and tracked jobs,
        match jobs to your CV, deliver any digests you subscribe to, and keep the platform secure and
        working. We do not sell your personal data or use it for third-party advertising.
      </p>
    </section>

    <section class="flex flex-col gap-3">
      <h2 class="text-xl font-semibold tracking-tight">Cookies</h2>
      <p class="text-sm leading-relaxed text-muted-foreground">
        When you sign in, we set a single <code class="font-mono text-foreground">HttpOnly</code>,
        <code class="font-mono text-foreground">SameSite=Lax</code> session cookie holding a signed
        token. It is strictly necessary for keeping you logged in and cannot be read by JavaScript.
        Logging out clears it. This cookie is always set and needs no consent.
      </p>
      <p class="text-sm leading-relaxed text-muted-foreground">
        For product analytics we use Google Analytics and PostHog, which set their own cookies and,
        in PostHog's case, may record a session replay with all inputs masked. These are
        non-essential. If you visit from the EU, EEA, or UK, they load only after you accept them in
        the cookie banner — reject and nothing loads. You can change your choice at any time via
        “Cookie settings” in the footer.
      </p>
    </section>

    <section class="flex flex-col gap-3">
      <h2 class="text-xl font-semibold tracking-tight">Job listings</h2>
      <p class="text-sm leading-relaxed text-muted-foreground">
        The jobs we display are aggregated from public company career boards and other public
        sources. We normalise and deduplicate them; we do not own this content, and a role closes in
        our catalogue once it disappears from its original source.
      </p>
    </section>

    <section class="flex flex-col gap-3">
      <h2 class="text-xl font-semibold tracking-tight">Third-party services</h2>
      <p class="text-sm leading-relaxed text-muted-foreground">
        We rely on a small set of processors to run freehire:
      </p>
      <ul class="flex flex-col gap-2 text-sm leading-relaxed text-muted-foreground">
        <li>
          <span class="font-medium text-foreground">A language-model provider</span> — processes CV
          and job text to produce AI fit analysis, only when you request it.
        </li>
        <li>
          <span class="font-medium text-foreground">Product analytics (Google Analytics, PostHog)</span>
          — measure aggregate usage to improve the product; loaded only with your consent where
          required (see “Cookies”). PostHog runs on its EU instance.
        </li>
        <li>
          <span class="font-medium text-foreground">Error monitoring (Sentry)</span> — captures
          application errors to keep the service reliable; configured without personal-data capture.
        </li>
        <li>
          <span class="font-medium text-foreground">OAuth providers (Google, GitHub, LinkedIn)</span>
          — only if you choose to sign in with them, to verify your identity and email.
        </li>
        <li>
          <span class="font-medium text-foreground">ChatGPT Actions</span> — if you connect freehire
          to a custom GPT, ChatGPT sends your search and tracking requests (authenticated with your
          API key) to our API. Your use of ChatGPT is also governed by OpenAI's own privacy policy.
        </li>
      </ul>
    </section>

    <section class="flex flex-col gap-3">
      <h2 class="text-xl font-semibold tracking-tight">Retention</h2>
      <p class="text-sm leading-relaxed text-muted-foreground">
        We keep account and activity data for as long as your account exists. When you delete your
        account, we delete or anonymise your personal data, except where we must keep it to comply
        with the law. Request logs are retained for a limited period for security and debugging.
      </p>
    </section>

    <section class="flex flex-col gap-3">
      <h2 class="text-xl font-semibold tracking-tight">Your rights</h2>
      <p class="text-sm leading-relaxed text-muted-foreground">
        You can access, correct, export, or delete your personal data at any time — most of it
        directly from your account settings, or by contacting us. You can also revoke API keys and
        unlink sign-in providers. If you are in the EU/EEA or UK, you have the rights granted by the
        GDPR, including the right to lodge a complaint with a supervisory authority.
      </p>
    </section>

    <section class="flex flex-col gap-3">
      <h2 class="text-xl font-semibold tracking-tight">Contact</h2>
      <p class="text-sm leading-relaxed text-muted-foreground">
        Questions or requests about your data? Reach us at
        <a
          href="mailto:{contactEmail}"
          class="font-medium text-foreground underline-offset-4 hover:underline">{contactEmail}</a
        >, on
        <a href={telegramUrl} class="font-medium text-foreground underline-offset-4 hover:underline"
          >Telegram</a
        >, or via
        <a href={repoUrl} class="font-medium text-foreground underline-offset-4 hover:underline"
          >GitHub</a
        >. As an open-source project, our data handling follows what the
        <a href={repoUrl} class="font-medium text-foreground underline-offset-4 hover:underline"
          >source code</a
        >
        actually does.
      </p>
    </section>
  </div>
</div>
