import type { Http } from '../api-core';
import type { components } from '../generated/api.js';
import type { OnboardingMatches, OnboardingParse } from '../types';
import { ApiError, jsonBody, RESUME_MAX_BYTES, RESUME_MAX_MB, resumeInit } from '../api-core';

/** What a 429 from the anonymous CV parse means: the gateway's per-address daily budget is
 *  spent (signed-in callers do not draw on it, so signing in is the way on). */
export const ONBOARDING_DAILY_LIMIT_MESSAGE = "You've hit today's limit for anonymous CV reads.";

export function onboardingApi({ request, requestData }: Http) {
  /** Read a PDF CV for the CV-first onboarding flow. Open to anonymous and signed-in
   *  callers; stores nothing. A 429 is the per-address daily limit and is rethrown with
   *  that meaning; any other refusal is an unreadable file, worded by the server. */
  async function onboardingParseCv(file: File): Promise<OnboardingParse> {
    if (file.size > RESUME_MAX_BYTES) {
      throw new ApiError(
        413,
        `This PDF is larger than ${RESUME_MAX_MB} MB. Compress it or export a lighter PDF and try again.`,
      );
    }
    try {
      return await requestData<OnboardingParse>('/api/v1/onboarding/cv', resumeInit('POST', file));
    } catch (e) {
      if (e instanceof ApiError && e.status === 429) throw new ApiError(429, ONBOARDING_DAILY_LIMIT_MESSAGE, e.body);
      throw e;
    }
  }

  /** The top open jobs for a skill list under `query` — a jobs-search query string with
   *  the ordinary filter param names — ranked by skill match. Public. */
  function onboardingMatches(skills: string[], query: string): Promise<OnboardingMatches> {
    return request<OnboardingMatches>(
      '/api/v1/onboarding/matches',
      jsonBody('POST', { skills, query } satisfies components['schemas']['OnboardingMatchesRequest']),
    );
  }

  return { onboardingParseCv, onboardingMatches };
}
