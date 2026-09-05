-- Raise the user profile's specialization cardinality bound from 5 to 10, the backstop for
-- the cap internal/identity/userprofile enforces (MaxSpecializations).
--
-- Why: 5 was reached without anyone choosing to reach it. A CV resolves several categories
-- on its own and both the profile form and the onboarding wizard UNION what an import
-- resolved into whatever is already picked, so a candidate whose work spans backend, data,
-- cloud and a couple more simply could not save — the onboarding wizard had no cap of its
-- own and swallowed the resulting 400, which read to the user as a wizard that does
-- nothing. The bound still exists: this is a profile's role list, not a catalogue.
--
-- The constraint is replaced rather than relaxed in place (Postgres has no ALTER
-- CONSTRAINT for a CHECK expression). Every existing row satisfies the wider bound by
-- construction — the old one only ever admitted 5 — so the recreate cannot fail on data.
--
-- Added VALID rather than 0056's NOT VALID, which was written for a bound that existing
-- rows might genuinely violate. This one cannot be violated, and the table it scans is the
-- profile table — a few hundred rows, not the catalogue — so the validation is part of the
-- same statement and the constraint does not carry NOT VALID forever for nothing.
--
-- Run BEFORE the binary that writes 6-10 specializations: the old constraint would reject
-- such a write, and the old binary never produces one, so either order is safe for the
-- code but only this one is safe for the user.
ALTER TABLE public.user_profiles
    DROP CONSTRAINT IF EXISTS user_profiles_specializations_card_chk;

ALTER TABLE public.user_profiles
    -- squawk-ignore constraint-missing-not-valid -- a few hundred profile rows, all of which already satisfy a STRICTLY tighter bound; the scan cannot fail and is not worth a permanent NOT VALID
    ADD CONSTRAINT user_profiles_specializations_card_chk
    CHECK ((cardinality(specializations) >= 1) AND (cardinality(specializations) <= 10));
