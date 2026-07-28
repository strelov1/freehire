-- Admit the 'profile' preset (see the experience-bank change).
--
-- 0044 pinned the preset vocabulary in a CHECK constraint, which is the right call — an
-- unknown preset would otherwise reach SystemPrompt, fall through to the chat prompt, and
-- run a session under instructions nobody chose. The consequence is that adding a preset
-- is a schema change and not just a Go constant, and a deploy that forgets this one fails
-- every attempt to start a profile session with a 23514.
--
-- The profile preset is the experience interviewer: it runs the same tool set as a chat
-- session and differs only in what its prompt goes looking for — a role with no
-- achievements recorded against it, a skill the candidate says they have with nothing to
-- show for it, an achievement with no number in it.
--
-- Applied to a fresh volume by initdb after 0047; on an existing prod volume run this
-- manually (SET ROLE hire) BEFORE deploying code that creates profile sessions.

ALTER TABLE public.assistant_sessions
    DROP CONSTRAINT assistant_sessions_preset_check;

ALTER TABLE public.assistant_sessions
    ADD CONSTRAINT assistant_sessions_preset_check
    CHECK ((preset = ANY (ARRAY['chat'::text, 'tailor'::text, 'profile'::text])));
