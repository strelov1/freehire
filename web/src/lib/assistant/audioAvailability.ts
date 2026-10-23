/** Which assistant surfaces that need a microphone are offered.
 *
 *  Both went dark on 2026-08-22, when the OpenAI-compatible gateway they reach the
 *  model through was replaced and its audio routes were unproven. Shipping the
 *  affordance before a route works means a microphone button that opens, records and
 *  then fails — the server answers 501 and `dictationOff` / `voiceModeOff` latch it
 *  away for the rest of the session. That is a worse first impression than no button.
 *
 *  These are deliberately constants and not configuration. Nothing about them varies
 *  per deployment: either the gateway serves a route or it does not, and the day it
 *  does, the matching flag flips to `true` in one edit and the capability checks
 *  underneath — `canRecord`, `canUseVoiceCall` — take over again untouched.
 *
 *  Dictation is back since 2026-10-23: `/v1/audio/transcriptions` was exercised
 *  against the gateway with the production configuration (`STT_MODEL`) and
 *  transcribed a recording correctly. */
export const DICTATION_ENABLED = true;

/** Hands-free voice mode stays off: `/v1/realtime/client_secrets` has still never been
 *  called against the gateway, and that gateway wants `session.model` as
 *  `provider/model`, which is not the shape sent today. To restore: verify the route,
 *  fix the shape if needed, then flip this. */
export const VOICE_CALL_ENABLED = false;
