-- beta_tester marks an account as a member of the beta-tester group — a rollout
-- gate that is deliberately SEPARATE from `role` (a user can be a moderator
-- and/or a beta tester, independently). It currently gates the in-app agent
-- assistant (/my/assistant): the page and its nav entry show only to beta
-- testers. Membership is granted out-of-band (manual SQL); there is no
-- self-service grant. Defaults false so existing accounts are non-members.
ALTER TABLE public.users
    ADD COLUMN beta_tester boolean DEFAULT false NOT NULL;
