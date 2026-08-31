<script lang="ts">
  import { page } from '$app/state';
  import Seo from '$lib/components/Seo.svelte';

  const contactEmail = 'hello@freehire.me';
  const canonical = $derived(`${page.url.origin}/delete-account`);
</script>

<Seo
  title="Delete Your Account — freehire"
  description="How to permanently delete your freehire account and data, on the web or in the mobile app."
  {canonical}
/>

<div class="mx-auto w-full max-w-3xl px-4 py-6">
  <div class="flex flex-col gap-8">
    <header class="flex flex-col gap-3">
      <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// legal</p>
      <h1 class="text-4xl font-semibold leading-[1.0] tracking-tighter sm:text-5xl">
        Delete Your Account
      </h1>
    </header>

    <p class="text-base leading-relaxed text-muted-foreground">
      You can permanently delete your freehire account and its data at any time, from the website
      or the mobile app. This cannot be undone — there is no grace period and no recovery.
    </p>

    <section class="flex flex-col gap-3">
      <h2 class="text-xl font-semibold tracking-tight">How</h2>
      <ol class="flex flex-col gap-2 text-sm leading-relaxed text-muted-foreground list-decimal pl-5">
        <li>Sign in, then go to <span class="font-medium text-foreground">Account settings</span> (Profile).</li>
        <li>Find <span class="font-medium text-foreground">Delete account</span> and open it.</li>
        <li>Type your account email to confirm, then confirm the deletion.</li>
      </ol>
      <p class="text-sm leading-relaxed text-muted-foreground">
        The same flow is available inside the mobile app, under Account settings.
      </p>
    </section>

    <section class="flex flex-col gap-3">
      <h2 class="text-xl font-semibold tracking-tight">What gets deleted</h2>
      <ul class="flex flex-col gap-2 text-sm leading-relaxed text-muted-foreground list-disc pl-5">
        <li>Your CV, its parsed profile, and every CV you built or tailored</li>
        <li>Your hosted mailbox, connected Gmail, and all stored messages</li>
        <li>Saved jobs, applications, reminders, and match analyses</li>
        <li>Your plan and its usage history, saved searches, filters, and API keys</li>
        <li>Your anonymous community handle</li>
        <li>Your account and login credentials</li>
      </ul>
      <p class="text-sm leading-relaxed text-muted-foreground">
        Deletion is immediate and permanent — we don't keep a recoverable copy afterward.
      </p>
    </section>

    <section class="flex flex-col gap-3">
      <h2 class="text-xl font-semibold tracking-tight">Can't sign in?</h2>
      <p class="text-sm leading-relaxed text-muted-foreground">
        If you can't access your account to delete it yourself, email us at
        <a
          href="mailto:{contactEmail}"
          class="font-medium text-foreground underline-offset-4 hover:underline">{contactEmail}</a
        >
        from the address on the account and we'll delete it for you.
      </p>
    </section>
  </div>
</div>
