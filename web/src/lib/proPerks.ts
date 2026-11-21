// What a paid plan adds, shown on /pricing and on the tailoring landing. The daily
// amounts come from the backend's plan matrix, so nothing here quotes a number.
import { resolve } from '$app/paths';

export const PRO_PERKS: { text: string; href: string }[] = [
  { text: 'Instant or batched saved-search alerts, not only a daily digest', href: resolve('/features/notifications') },
  { text: 'Daily hiring inbox summaries', href: resolve('/features/inbox') },
  { text: 'CV tailoring for each vacancy, with cover letters', href: resolve('/features/tailor') },
  { text: 'Recruiter contacts with work email on job pages', href: resolve('/features/recruiter-contacts') },
  { text: 'AI assistant', href: resolve('/features/tailor') },
  { text: 'Chrome and Firefox extension that fills applications for you', href: resolve('/features/extension') },
  { text: 'Auto-apply in the background, within a daily limit', href: resolve('/features/extension') },
];
