## REMOVED Requirements

### Requirement: A settled turn offers up to three follow-up questions

**Reason**: The Follow-ups feature is discontinued by product decision — it fired after
nearly every exchange in real sessions and added noise without earning its keep.
**Migration**: None. No replacement suggests what to ask next; the composer is the only
way to continue a conversation.

### Requirement: Only the newest answer carries follow-ups

**Reason**: The feature that scoped follow-ups to the newest answer no longer exists.
**Migration**: None.

### Requirement: Clicking a follow-up sends it as an ordinary message

**Reason**: There is no follow-up chip left to click.
**Migration**: None — the ordinary composer remains the only way to send a message.

### Requirement: Follow-ups render as inert text

**Reason**: There is no follow-up content left to render.
**Migration**: None.

### Requirement: Failing to produce follow-ups is invisible

**Reason**: There is no follow-up generation left to fail.
**Migration**: None.

### Requirement: The follow-up endpoint is owner-scoped

**Reason**: `POST /api/v1/assistant/sessions/:id/followups` is deleted along with the
rest of the feature.
**Migration**: None — no client should call this endpoint after this change ships; any
existing caller receives 404 like any other removed route.
