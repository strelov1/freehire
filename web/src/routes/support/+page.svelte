<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import Seo from '$lib/components/Seo.svelte';

  const repoUrl = 'https://github.com/strelov1/freehire';
  const issuesUrl = 'https://github.com/strelov1/freehire/issues';
  const telegramUrl = 'https://t.me/freehiredev';
  const contactEmail = 'hello@freehire.me';

  const canonical = $derived(`${page.url.origin}/support`);

  const link = 'font-medium text-foreground underline-offset-4 hover:underline';
</script>

<Seo
  title="Support — freehire"
  description="Help with freehire on the web and in the mobile app: how the profile match works, managing your subscription, deleting your account, and how to reach us."
  {canonical}
/>

<div class="mx-auto w-full max-w-3xl px-4 py-6">
  <div class="flex flex-col gap-8">
    <header class="flex flex-col gap-3">
      <p class="font-mono text-xs uppercase tracking-widest text-muted-foreground">// support</p>
      <h1 class="text-4xl font-semibold leading-none tracking-tighter sm:text-5xl">Support</h1>
    </header>

    <p class="text-base leading-relaxed text-muted-foreground">
      Something not working, or not behaving the way you expected? Write to
      <a href="mailto:{contactEmail}" class={link}>{contactEmail}</a> — a person reads it. For
      anything that looks like a bug, an issue on
      <a href={issuesUrl} class={link}>GitHub</a> is even better: freehire is open source, so you can
      watch the fix happen.
    </p>

    <section class="flex flex-col gap-3">
      <h2 class="text-xl font-semibold tracking-tight">Get in touch</h2>
      <ul class="flex flex-col gap-2 text-sm leading-relaxed text-muted-foreground">
        <li>
          <span class="text-foreground">Email</span> —
          <a href="mailto:{contactEmail}" class={link}>{contactEmail}</a>. Best for anything about
          your account, your data, or a payment.
        </li>
        <li>
          <span class="text-foreground">GitHub issues</span> —
          <a href={issuesUrl} class={link}>report a bug or request a feature</a>. Public, and the
          fastest route to a fix.
        </li>
        <li>
          <span class="text-foreground">Telegram</span> —
          <a href={telegramUrl} class={link}>@freehiredev</a>, for questions and release notes.
        </li>
      </ul>
    </section>

    <section class="flex flex-col gap-3">
      <h2 class="text-xl font-semibold tracking-tight">The profile match</h2>
      <div class="flex flex-col gap-3 text-sm leading-relaxed text-muted-foreground">
        <p>
          <span class="text-foreground">Why does a job show no match percentage?</span> The match is
          computed against the skills in your profile, so it needs two things: you signed in, and your
          profile lists at least one skill. A job that lists no recognised skills has nothing to
          compare either — it says so rather than showing 0%.
        </p>
        <p>
          <span class="text-foreground">What does "Close" mean?</span> A skill you do not have, but
          which a skill you DO have is adjacent to — the chip names it, so
          <code class="rounded bg-muted px-1 py-0.5 text-xs">azure · aws</code> reads as "the job wants
          Azure; you have AWS". A close match counts half of an exact one, which is why the percentage
          moves less than you might expect when one appears.
        </p>
        <p>
          <span class="text-foreground">The match is wrong about a skill I have.</span> Tap the chip
          and add it to your profile. It counts immediately, on this job and every other one. Tapping
          a chip also lets you mark a skill you would rather not be shown.
        </p>
        <p>
          <span class="text-foreground">What are the "Requirements" under the match?</span> Deterministic
          checks against your CV — years of experience, education, languages, work authorisation. They
          are advisory: they never hide a job or push it down the list.
        </p>
      </div>
    </section>

    <section class="flex flex-col gap-3">
      <h2 class="text-xl font-semibold tracking-tight">Jobs and postings</h2>
      <div class="flex flex-col gap-3 text-sm leading-relaxed text-muted-foreground">
        <p>
          <span class="text-foreground">What does "Open 75d" mean on a card?</span> How long the
          posting has actually been open, when that is longer than its date suggests. freehire also
          marks a posting that has been reposted, or that has several live copies — the signals that
          usually mean a role is not really being filled.
        </p>
        <p>
          <span class="text-foreground">A job I applied to is gone.</span> Postings disappear when
          the source removes them. Anything you saved or tracked stays in your tracker with the
          details as they were.
        </p>
        <p>
          <span class="text-foreground">The apply button opens someone else's site.</span> That is
          the employer's own application form. freehire aggregates postings; it does not receive
          applications, and never applies on your behalf.
        </p>
      </div>
    </section>

    <section class="flex flex-col gap-3">
      <h2 class="text-xl font-semibold tracking-tight">Account and subscription</h2>
      <div class="flex flex-col gap-3 text-sm leading-relaxed text-muted-foreground">
        <p>
          <span class="text-foreground">I cannot sign in.</span> If you first signed up with Google,
          GitHub, LinkedIn or Apple, use that same button rather than a password — the account has no
          password until you set one. If you did set one, use "Forgot password". Still stuck? Email us
          from the address on the account.
        </p>
        <p>
          <span class="text-foreground">How do I cancel Pro?</span> Where you bought it. A subscription
          bought in the iPhone app is cancelled in
          <a href="https://apps.apple.com/account/subscriptions" class={link}>Apple's subscription
            settings</a>; one bought on the web is cancelled on your
          <a href={resolve('/my/plan')} class={link}>plan page</a>. Cancelling stops the renewal — the
          plan stays active until the period you already paid for ends.
        </p>
        <p>
          <span class="text-foreground">A charge I do not recognise.</span> Refunds for an in-app
          purchase are handled by Apple, not by us:
          <a href="https://reportaproblem.apple.com" class={link}>reportaproblem.apple.com</a>. For a
          card payment made on the web, email us and we will sort it out.
        </p>
        <p>
          <span class="text-foreground">How do I delete my account?</span> From the app or the
          website, in a few taps — see
          <a href={resolve('/delete-account')} class={link}>deleting your account</a>. It is permanent:
          no grace period, no recovery.
        </p>
      </div>
    </section>

    <section class="flex flex-col gap-3">
      <h2 class="text-xl font-semibold tracking-tight">The mobile app</h2>
      <div class="flex flex-col gap-3 text-sm leading-relaxed text-muted-foreground">
        <p>
          <span class="text-foreground">Notifications are not arriving.</span> Check that
          notifications are allowed for freehire in your device settings, and that the device is
          listed on your account. Signing out removes that device's registration.
        </p>
        <p>
          <span class="text-foreground">The app and the website disagree.</span> They share one
          account and one database, so this is almost always a stale screen — pull to refresh. If it
          persists, tell us what each one showed.
        </p>
      </div>
    </section>

    <section class="flex flex-col gap-3">
      <h2 class="text-xl font-semibold tracking-tight">Also useful</h2>
      <ul class="flex flex-col gap-2 text-sm leading-relaxed text-muted-foreground">
        <li><a href={resolve('/privacy')} class={link}>Privacy Policy</a> — what we collect and why.</li>
        <li><a href={resolve('/terms')} class={link}>Terms of Service</a>.</li>
        <li>
          <a href={resolve('/delete-account')} class={link}>Deleting your account</a> — how, and what
          it removes.
        </li>
        <li>
          <a href={resolve('/how-it-works')} class={link}>How freehire works</a> — where the jobs come
          from and how they are deduplicated.
        </li>
        <li><a href={repoUrl} class={link}>Source code</a> — the honest answer to "what does it do with my data".</li>
      </ul>
    </section>
  </div>
</div>
