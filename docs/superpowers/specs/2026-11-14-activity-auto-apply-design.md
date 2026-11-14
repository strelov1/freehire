# Auto-apply in Activity

## Intent

Give a signed-in candidate one place to find their automatic applications, especially
attempts awaiting review or stopped by a form or a resume preparation failure.

## Approach

Add an Auto-apply tab at `/my/activity/auto-apply`, alongside Saved, History, Matches and
Hidden. A separate tab matches the requested separate list and the section's existing
navigation. Adding these attempts to History would mix browsing with application progress;
putting them only in Tracking would leave Activity without the requested entry point.

Reuse the existing auto-apply listing and status derivation behind the assistant's
`auto_apply_list` through an authenticated HTTP endpoint. The view is scoped to the caller;
it is not an administrative list of all candidates. No migration is needed.

## View

Show the vacancy, employer, readable status and last activity time, newest activity first.
Include all active attempts and successful submissions from the last 30 days, matching
the existing listing's retention. Explain that successful submissions cover this window.
Distinguish preparation, waiting for review, approved, blocked, declined, submission failure,
resume preparation failure and submitted. Use existing Activity layout and design tokens.

Each row opens the existing application detail surface for the job, where the candidate
can inspect missing answers, review the prepared application, retry or cancel when allowed.
Do not add immediate submission or bulk actions to the list. Do not expose raw internal
errors. Provide loading, error with retry, and empty states, and existing English/Russian
localization. Preserve account authentication and the existing server-side action guards.

## Validation

Check authentication and caller isolation on the list endpoint, status consistency with
the existing list, submitted entries, and the empty result. Check the new view's loading,
error, empty and populated states, navigation, mobile layout and type checking.

## Scope

This change makes existing attempts discoverable. Repairing blocked applications, changing
worker scheduling, sending applications, and deployment are outside this change.
