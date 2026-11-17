import { track } from '#lib/analytics.js';
import { api } from '#lib/api.js';
import type { CvRecord, TailorResult } from '#lib/cv.js';
import type { Analysis } from '#lib/generated/contracts.js';
import type { Job } from '#lib/types.js';

/** What opening the workspace found. `tailor` is set only on a bootstrap (the CV is not in
 *  hand yet); otherwise `cv` is the record to hydrate and `resuming` is true. */
export type OpenedWorkspace = {
  cvId: string;
  cv?: CvRecord;
  tailor?: TailorResult;
  sessionId?: string;
  job?: Job;
  analysis?: Analysis | null;
  resuming: boolean;
};

/** The three ways in: a base CV (`baseCvId`), a tailored CV by id (`cvParam`), or a bootstrap
 *  of the vacancy's tailored CV. Reads and starts sessions; touches no page state. */
export async function openWorkspace(args: {
  slug: string;
  baseCvId?: string;
  cvParam: string | null;
  baseParam?: string;
}): Promise<OpenedWorkspace> {
  const { slug, baseCvId, cvParam, baseParam } = args;
  if (baseCvId !== undefined) {
    // BASE mode: the CV is the address, there is no vacancy to fetch and nothing to tailor.
    // The conversation is opened here and costs nothing (see StartBaseSession): a free
    // account still edits by hand, and the chat answers its first message with the same
    // upgrade state the tailoring workspace shows.
    const [cv, base] = await Promise.all([api.getCv(baseCvId), api.startBaseSession(baseCvId)]);
    return { cvId: baseCvId, cv, sessionId: base.session_id, resuming: true };
  }
  if (cvParam) {
    // Resume an existing tailored CV. If it already has a bound session, re-attach it with
    // no kickoff. If it has none (a CV created before session binding), mint a fresh
    // tailoring session for it and let the kickoff orient the agent.
    const [job, fit] = await Promise.all([api.getJob(slug), api.getMatchAnalysis(slug).catch(() => null)]);
    const cv = await api.getCv(cvParam);
    const opened = { cvId: cvParam, cv, job, analysis: fit?.analysis ?? null, resuming: true };
    if (cv.agent_session_id) {
      // Resume with an existing session never hits POST /me/cvs/tailor, which is what
      // places the vacancy on the Tracking board. Re-run bootstrap for that side effect
      // only (idempotent: same CV, no second debit). Fire-and-forget so a slow tracking
      // write cannot block the workspace.
      void api.tailorCv(slug, baseParam).catch(() => {});
      return { ...opened, sessionId: cv.agent_session_id };
    }
    // A CV created before session binding: the backend mints a tailoring conversation bound
    // to it and stores it on the CV (and places the vacancy on the board).
    const s = await api.startTailorSession(cvParam);
    return { ...opened, sessionId: s.session_id };
  }
  // Bootstrap: reach the tailored CV for this vacancy (the backend returns the existing
  // one when there is one) and the conversation bound to it.
  // Only the bootstrap is tracked as a run: it is the path that mints a tailored
  // CV and spends one of the day's sessions. Re-opening an existing one
  // (startTailorSession above) costs nothing and would inflate the count on every
  // revisit.
  track('tailor_run', { slug });
  const [job, tailor] = await Promise.all([api.getJob(slug), api.tailorCv(slug, baseParam)]);
  return { cvId: '', tailor, job, resuming: false };
}
