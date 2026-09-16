<script lang="ts">
  import { page } from '$app/state';
  import Seo from '$lib/components/Seo.svelte';

  const repoUrl = 'https://github.com/strelov1/freehire';
  const telegramUrl = 'https://t.me/freehiredev';
  // Confirmed monitored. This is the address a data-subject request and the Google OAuth
  // review team both arrive at, so it has to stay a mailbox somebody actually reads —
  // the same string is published on /terms, /support and /delete-account.
  const contactEmail = 'hello@freehire.me';

  const canonical = $derived(`${page.url.origin}/privacy`);

  // Static effective date — freehire has no Date.now-driven content, and a hard
  // date is what a privacy policy needs. Bump this whenever the policy changes.
  const lastUpdated = 'September 16, 2026';
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
        href="https://freehire.me"
        class="font-medium text-foreground underline-offset-4 hover:underline">freehire.me</a
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
          provider's access tokens. Connecting Gmail or Google Calendar is a separate, opt-in step
          that does store a token — see “Gmail and Google Calendar”.
        </li>
        <li>
          <span class="font-medium text-foreground">Job activity.</span> If you save, view, apply to,
          or track a job, we store that interaction (the job, timestamps, application stage, and any
          notes you add) so we can show you your pipeline.
        </li>
        <li>
          <span class="font-medium text-foreground">CV / résumé.</span> If you upload a CV for skill
          matching or AI match analysis, we store the file and the text we extract from it. Running an
          AI match analysis sends the relevant parts of your CV, together with the job posting, to our
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
          and job text to produce AI match analysis, only when you request it; and, if you have
          connected Gmail, classifies the hiring mail our own keyword vocabulary could not place
          (see “Gmail and Google Calendar”). It processes this text to answer that one request and
          is contractually barred from training on it.
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
      <h2 class="text-xl font-semibold tracking-tight">Browser extension</h2>
      <p class="text-sm leading-relaxed text-muted-foreground">
        The freehire Chrome extension puts a job-application agent in a side panel next to whatever
        page you are on. It does nothing until you sign in from the panel.
      </p>
      <ul class="flex flex-col gap-3 text-sm leading-relaxed text-muted-foreground">
        <li>
          <span class="font-medium text-foreground">Session token.</span> Signing in stores your
          freehire session token in <code class="font-mono text-foreground">chrome.storage.local</code
          >, scoped to your browser profile. Nothing else is stored there.
        </li>
        <li>
          <span class="font-medium text-foreground">Page content.</span> While the panel is open and
          only in service of what you asked for, it can read the current page's URL, title, and
          visible text (capped at 5,000 characters), or the fields of a job-application form you asked
          it to fill. This is sent to freehire.me — the only host the extension talks to — and kept in
          that conversation's transcript, which you can read and delete from your account. A read is
          always named in the panel, and browser-internal pages, other extensions' pages, and local
          files are never read.
        </li>
        <li>
          <span class="font-medium text-foreground">Profile data for Autofill.</span> Filling an
          application form sends the relevant fields from your freehire profile (name, email, phone,
          CV fields) to the page; you review and submit yourself.
        </li>
      </ul>
      <p class="text-sm leading-relaxed text-muted-foreground">
        We do not sell this data, and we do not use it for anything unrelated to running the
        extension's job-application agent, in line with the Chrome Web Store's limited-use
        requirements.
      </p>
    </section>

    <section class="flex flex-col gap-3">
      <h2 class="text-xl font-semibold tracking-tight">Gmail and Google Calendar</h2>
      <p class="text-sm leading-relaxed text-muted-foreground">
        This section applies only if you use the Inbox feature and connect a Google account to it.
        It is off by default, it is separate from signing in with Google, and nothing below happens
        for anyone who has not connected one. You can disconnect at any time from your account
        settings, or revoke the grant directly in your
        <a
          href="https://myaccount.google.com/permissions"
          class="font-medium text-foreground underline-offset-4 hover:underline"
          >Google account permissions</a
        >.
      </p>
      <ul class="flex flex-col gap-3 text-sm leading-relaxed text-muted-foreground">
        <li>
          <span class="font-medium text-foreground">What we read</span> (<code
            class="font-mono text-foreground">gmail.readonly</code
          >). We do not read your whole mailbox. Each sync runs a search for hiring-shaped mail
          only — messages from recognised applicant-tracking systems, or carrying application and
          interview wording — and skips mail you sent yourself. We store the matching messages
          (sender, subject, date, body) so your application inbox and pipeline stages work. Nothing
          else in your mailbox is fetched, and we never send mail on your behalf.
        </li>
        <li>
          <span class="font-medium text-foreground">Your calendar</span> (<code
            class="font-mono text-foreground">calendar.readonly</code
          >, and <code class="font-mono text-foreground">calendar.events</code> only if you are a
          mentor taking bookings). Read access shows your interviews alongside your applications.
          The mentor write scope creates the event and Meet link for a session someone books with
          you — we create nothing else and delete nothing.
        </li>
        <li>
          <span class="font-medium text-foreground">Automated processing.</span> To decide which
          application a message belongs to and what stage it signals, we first try a fixed keyword
          vocabulary that runs entirely on our own servers. Only when that cannot decide do we send
          the sender, the subject, and the first 4,000 characters of the body to our language-model
          provider (see “Third-party services”). No human at freehire reads your mail, except where
          you explicitly ask us to look at a specific message for support, or where we are legally
          required to.
        </li>
        <li>
          <span class="font-medium text-foreground">Your Google token.</span> The refresh token that
          lets us sync is encrypted at rest with AES-256 and is never shown to you or to anyone
          else. Disconnecting revokes it with Google and deletes the messages we synced from that
          account.
        </li>
      </ul>
      <p class="text-sm leading-relaxed text-muted-foreground">
        freehire's use and transfer to any other app of information received from Google APIs will
        adhere to the
        <a
          href="https://developers.google.com/terms/api-services-user-data-policy"
          class="font-medium text-foreground underline-offset-4 hover:underline"
          >Google API Services User Data Policy</a
        >, including the Limited Use requirements. We do not sell this data, we do not use it for
        advertising or to build advertising profiles, we do not use it to train or improve any
        generalised machine-learning model, and we do not transfer it to anyone except the
        processors named in this policy and only to provide the Inbox feature you asked for.
      </p>
    </section>

    <section class="flex flex-col gap-3">
      <h2 class="text-xl font-semibold tracking-tight">CV link tracking</h2>
      <p class="text-sm leading-relaxed text-muted-foreground">
        You can turn on link tracking for a single CV. It is off for every CV unless you switch it
        on, and switching it on for one CV does not affect any other.
      </p>
      <p class="text-sm leading-relaxed text-muted-foreground">
        When it is on, the links in that CV's PDF point at freehire and forward to the real
        destination. Following one records the time, the browser and operating system family, the
        device type, and the host — not the full address — of the page the visitor came from. It
        also records a keyed hash of the visitor's IP address and browser identity, so that repeat
        visits can be told from separate people. We do not store the IP address itself, and the
        hash is keyed with a secret so it cannot be turned back into an address.
      </p>
      <p class="text-sm leading-relaxed text-muted-foreground">
        These records are deleted after 180 days, and immediately if you delete the CV. Your own
        clicks are marked as yours and left out of the counts. A recorded open means the link was
        fetched; company mail systems follow links automatically, so it is not proof that a person
        read your CV.
      </p>
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
