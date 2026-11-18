/**
 * TailorWorkspace's props. Two modes, one component. With `slug` it is the tailoring workspace for a vacancy. With
 * `baseCvId` it is the BASE mode: the candidate's general CV, with no vacancy — so none of the
 * vacancy panes (job, match analysis, ATS delta against the base, cover letter, the autopilot
 * run) — while the preview, the section editor, template/style/margins, the experience bank,
 * the revision history with undo and the agent chat stay. The agent there runs under the `base`
 * preset: the same CV tools and evidence gate, a prompt that edits the general CV.
 */
export type TailorWorkspaceProps = { slug?: string; baseCvId?: string };
