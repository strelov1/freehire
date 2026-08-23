/** Whether the assistant offers anything that needs a microphone.
 *
 *  Off since 2026-08-22. Both audio surfaces — dictation into the composer and
 *  hands-free voice mode — reach the model through an OpenAI-compatible gateway,
 *  and the gateway is being replaced (see `provision/bifrost/` in freehire-ops).
 *  The replacement's audio routes exist but have never been exercised: the key
 *  pool behind it carries no OpenAI credential, so `/v1/audio/transcriptions`
 *  and `/v1/realtime/client_secrets` are unproven rather than known-good.
 *
 *  Shipping the affordance anyway would mean a microphone button that opens,
 *  records, and then fails — the server answers 501 and `dictationOff` /
 *  `voiceModeOff` latch it away for the rest of the session. That is a worse
 *  first impression than no button.
 *
 *  This is deliberately a constant and not configuration. Nothing about it
 *  varies per deployment: either the gateway serves audio or it does not, and
 *  the day it does, this flips to `true` in one edit and the capability checks
 *  underneath — `canRecord`, `canUseVoiceCall` — take over again untouched.
 *
 *  To restore: give the gateway an audio-capable credential, verify both routes
 *  against it, then flip this. Nothing else here was removed. */
export const AUDIO_ENABLED = false;
