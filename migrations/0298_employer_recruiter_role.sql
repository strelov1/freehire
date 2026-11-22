-- Keep the capabilities of existing members and already-issued invitations. The
-- invitation write explicitly selects recruiter for every new or replaced token.
ALTER TABLE public.company_account_members
    ADD COLUMN role text NOT NULL DEFAULT 'manager' CHECK (role IN ('manager', 'recruiter'));

ALTER TABLE public.company_account_invitations
    ADD COLUMN role text NOT NULL DEFAULT 'manager' CHECK (role IN ('manager', 'recruiter'));
