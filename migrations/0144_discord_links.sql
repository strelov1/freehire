-- The binding between a freehire account and a Discord account, so the paid role on the
-- community server can follow the subscription.
--
-- A table called discord_links existed before and was dropped in 0134 with the contribution
-- bot it served. Nothing carries over — that one mapped a user to a Discord id so a slash
-- command could attribute a submission, and its rows were archived on the host before the
-- drop. The name is reused because it is still the right name.
--
-- WHY discord_user_id IS text AND NOT bigint. A Discord snowflake is transported as a
-- decimal STRING in every payload the API returns, and its width is not contractually fixed
-- at 64 bits. The 0134 table stored it as bigint; that worked and was still a bet on an
-- undocumented bound, taken for no gain — nothing here does arithmetic on the value. text
-- also means a malformed id fails at the API boundary where it can be reported, rather than
-- as a parse error deep in a worker.
--
-- WHY discord_user_id IS UNIQUE. This is the constraint the feature rests on. Without it two
-- freehire accounts may name one Discord account, and a single subscription then confers the
-- role on somebody who never bought it — the whole gate, defeated by pasting a friend's
-- consent. It is enforced here and not only in the service because the service is not the
-- only writer a future change might add.
--
-- WHY user_id IS THE PRIMARY KEY. One account links one Discord identity. A second link
-- would be a second door into the same paid channels for the same subscription.
--
-- ON DELETE CASCADE: a deleted account holds no subscription, so its binding is meaningless.
-- The role it may still hold is revoked before deletion by the unlink path; a row surviving
-- its user would instead be a link the reconciliation could never resolve a plan for.
CREATE TABLE public.discord_links (
    user_id         bigint      NOT NULL PRIMARY KEY REFERENCES public.users(id) ON DELETE CASCADE,
    discord_user_id text        NOT NULL UNIQUE,
    linked_at       timestamptz NOT NULL DEFAULT now(),
    role_granted_at timestamptz,
    synced_at       timestamptz
);

COMMENT ON TABLE public.discord_links IS
    'One freehire account bound to one Discord account, so cmd/discord-sync can keep the '
    'paid role in step with the subscription. Holds no token: the OAuth access token is '
    'used inside the callback request and never stored.';

COMMENT ON COLUMN public.discord_links.role_granted_at IS
    'When the paid role was last granted, or NULL when the role is not currently held. It '
    'exists so reconciliation can tell "never granted" from "granted, now due for revocation" '
    'without asking Discord about every account on every run.';

COMMENT ON COLUMN public.discord_links.synced_at IS
    'When reconciliation last examined this row, or NULL if never. Reconciliation orders by '
    'it NULLS FIRST, which turns a bounded run into a rotating queue: a run that cannot '
    'reach everybody still reaches everybody over successive runs, with no cursor to store '
    'and nothing to reset when a run stops early.';

-- The reconciliation page's ordering. Small table (one row per linked subscriber), so this
-- is about determinism under the bound rather than about scan cost: without it the planner
-- is free to return the same accounts every hour and the rest are never synced at all.
CREATE INDEX discord_links_synced_at_idx ON public.discord_links (synced_at NULLS FIRST, user_id);
