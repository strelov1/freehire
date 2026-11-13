## REMOVED Requirements

### Requirement: Tour is shown once and only asks what is missing
**Reason**: The seven-step post-signup tour is replaced by the CV-first flow, which shows personal results before asking for an account.
**Migration**: See `cv-first-onboarding`. An account whose onboarding is not complete is still routed to `/onboarding`, which now runs the CV-first flow.

### Requirement: Stored answers pre-fill the tour
**Reason**: The tour is removed; the CV-first flow derives its answers from the uploaded CV and three one-click questions.
**Migration**: See `cv-first-onboarding`; stored profile values remain editable in the profile pages.

### Requirement: CV step pre-fills feed preferences
**Reason**: Replaced by the CV-first flow's parse and questions.
**Migration**: See `cv-first-onboarding` (Requirement: Signing up keeps what the flow found).

### Requirement: Feed step saves the search profile and shows its result
**Reason**: Replaced by the CV-first flow, which saves the search profile from the CV and answers at sign-up.
**Migration**: The full specialization/skills/location editor remains in the profile pages.

### Requirement: Alert channels step
**Reason**: The CV-first flow subscribes the profile alert over email at sign-up; other channels are set up from the notification settings.
**Migration**: Telegram and push for the profile alert stay available in the notification settings.

### Requirement: Challenge answers choose what helps
**Reason**: The challenge question moves into the CV-first flow as its last question.
**Migration**: See `cv-first-onboarding` (Requirement: Three quick questions): same vocabulary, same exclusivity of "just starting", saved to the survey at sign-up. The tips move to the parked product introduction page.

### Requirement: Pro steps describe the paid features
**Reason**: The tailoring and extension introductions leave onboarding.
**Migration**: See `cv-first-onboarding` (Requirement: Product introduction is parked in the product); feature pages and `/pricing` are unchanged.
