import { ApiError } from './api-core';

interface SearchRateLimit {
  title: string;
  description: string;
  upgrade: boolean;
}

export function searchRateLimit(error: unknown): SearchRateLimit | null {
  if (!(error instanceof ApiError) || error.status !== 429) return null;
  const generic = {
    title: 'Too many search requests',
    description: 'Please wait a moment before searching again.',
    upgrade: false,
  };
  const body = error.body;
  if (
    body?.code !== 'rate_limited' ||
    body.budget !== 'publicread' ||
    typeof body.limit !== 'number' ||
    !Number.isInteger(body.limit) ||
    body.limit <= 0 ||
    typeof body.retry_after_seconds !== 'number' ||
    !Number.isInteger(body.retry_after_seconds) ||
    body.retry_after_seconds <= 0 ||
    body.retry_after_seconds > 86400
  )
    return generic;

  const names: Record<string, string> = {
    anonymous: 'guest',
    free: 'Free',
    pro: 'Pro',
    ultra: 'Ultra',
    api: 'API',
    api_data: 'API Data',
  };
  const tier = typeof body.tier === 'string' ? names[body.tier] : undefined;
  if (!tier) return generic;
  const upgrade = body.upgrade_available === true && (body.tier === 'free' || body.tier === 'anonymous');
  const seconds = body.retry_after_seconds;
  return {
    title: 'Search limit reached',
    description: `Your ${tier} search limit is ${body.limit} requests per minute. Try again in ${seconds} ${seconds === 1 ? 'second' : 'seconds'}${upgrade ? ', or upgrade for a higher search limit.' : '.'}`,
    upgrade,
  };
}
