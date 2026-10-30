## Why

The account area grew one section at a time and now lists seventeen of them in a single
flat column: a candidate's own profile, the anonymous Talent Network card, the anonymous
interview-buddy handle, the employer's company page, the mentor's schedule and the account
settings all sit side by side as equals. Nothing tells a member which of these is *them*,
which is an anonymous face other people see, and which is a separate role they took on.
The same list is repeated in the header's profile menu, so it is long there too.

A member has one account and several faces. The navigation should say so.

## What Changes

- **Workspaces.** The account area is split into four workspaces — **Job search** (the
  default), **Mentor**, **Referrer** and **Employer**. The current workspace is read from the
  URL, so a link or a reload lands in the right one.
- **The sidebar shows only the current workspace**, grouped:
  - Job search: the everyday sections (Profile, Activity, Tracking, Inbox, Market Pulse,
    Agent, Tailor), then **Get found · anonymous** (Talent Network, Interview buddies), then
    **Get help** (Referral requests, Mentorship).
  - Mentor: Booked with you, Schedule, Mentor profile.
  - Referrer: Offers to refer, Incoming requests.
  - Employer: Company profile, Vacancies.
  - Every workspace ends with the same **Settings** group (Notifications, Integrations,
    Contributions, Plan, Account).
- **Pages that mixed two roles are split.** Referrals was one page with three tabs — the
  member's own requests and the insider's offers and inbox. The tabs become routes:
  `/my/referrals` (requests), `/my/referrals/offers`, `/my/referrals/incoming`; the old
  `?tab=offers|incoming` links (the referrer notification email carries one) redirect. The
  employer dashboard's two tabs become `/my/employer` and `/my/employer/vacancies`.
- **The profile menu becomes a workspace switcher** — Job search / Mentor / Employer, then
  Settings, Submit a job, Moderation (moderators) and Log out — instead of repeating every
  section. The phone menu's account section does the same.
- **The mentorship tab strip is removed**: the mentor's panes are sidebar items of the
  Mentor workspace now, and `/my/mentorship` itself is the member's own booked sessions,
  under Job search.
- **The Talent Network invitation shows only in Job search** — it is a candidate's prompt
  and reads as noise on the employer page.

Visibility is unchanged: Mentor is offered to beta testers only (the marketplace is still
beta-gated); Referrer and Employer to everyone (offering a referral and claiming a company
are both open). Offering a
workspace only once its role is active needs the account payload to say so; that is a
later change.

Frontend-only (`web/`). No backend, API or database change; every existing URL keeps
working.

## Capabilities

### Modified Capabilities

- `account-navigation`: section items become grouped and workspace-scoped; the rail lists
  the Job search workspace.
- `header-navigation`: the desktop profile menu and the phone menu's account section list
  workspaces instead of sections.

## Impact

- `web/src/lib/accountNav.ts` (+ test): items gain a workspace and a group; new
  `workspaceFor`, `workspaceNav`, `activeSectionHref`.
- `web/src/lib/accountNavIcons.ts`, `web/src/lib/i18n/shell.ts`: icons and labels for the
  new mentor items and the workspace/group names.
- `web/src/routes/my/+layout.svelte`, `AccountNavDrawer.svelte`, `AccountNavRail.svelte`:
  render the grouped workspace.
- `HeaderProfileMenu.svelte`, `HeaderMenu.svelte` (+ tests): the switcher.
- `web/src/routes/my/mentorship/+layout.svelte`: the tab strip goes; `mentorshipTabs.ts`
  goes with it if nothing else reads it.
- `ReferralsView.svelte` + new `routes/my/referrals/{offers,incoming}`;
  `EmployerView.svelte`/`EmployerDashboard.svelte` + new `routes/my/employer/vacancies`.
