import { FACET_ALIASES } from './generated/contracts';

// Search vocabulary can be broader than the title phrases used to classify vacancies.
export const SPECIALIZATION_ALIASES: Record<string, readonly string[]> = {
  ...FACET_ALIASES,
  network_engineering: [...FACET_ALIASES.network_engineering, 'computer networking', 'computer networks'],
  it_support: [...FACET_ALIASES.it_support, 'network support', 'computer support'],
  devops: [...FACET_ALIASES.devops, 'cloud computing'],
  security: [...FACET_ALIASES.security, 'cybersecurity soc analyst', 'network security engineering'],
};
