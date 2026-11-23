ALTER TABLE users ADD COLUMN interview_scheduling_enabled_at timestamptz;

CREATE TABLE interview_scheduling_threads (
    id                    bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id               bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    application_id        bigint NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    anchor_email_id       bigint NOT NULL REFERENCES emails(id) ON DELETE CASCADE,
    gmail_thread_id       text NOT NULL,
    recruiter_email       text NOT NULL,
    status                text NOT NULL DEFAULT 'new'
                          CHECK (status IN ('new', 'waiting', 'calendar', 'booked', 'review', 'closed')),
    last_processed_id     text NOT NULL DEFAULT '',
    last_sent_id          text NOT NULL DEFAULT '',
    offered_starts        timestamptz[] NOT NULL DEFAULT '{}',
    offer_zone            text NOT NULL DEFAULT '',
    language              text NOT NULL DEFAULT 'en',
    followups             bigint NOT NULL DEFAULT 0 CHECK (followups BETWEEN 0 AND 2),
    followup_at           timestamptz,
    selected_start        timestamptz,
    next_at               timestamptz NOT NULL DEFAULT now(),
    pending_body          text NOT NULL DEFAULT '',
    pending_parent_id     text NOT NULL DEFAULT '',
    pending_message_id    text NOT NULL DEFAULT '',
    pending_kind          text NOT NULL DEFAULT ''
                          CHECK (pending_kind IN ('', 'offer', 'followup', 'confirm')),
    attempted_at          timestamptz,
    UNIQUE (user_id, gmail_thread_id)
);
CREATE INDEX interview_scheduling_application_idx ON interview_scheduling_threads(application_id);
CREATE INDEX interview_scheduling_anchor_idx ON interview_scheduling_threads(anchor_email_id);
CREATE INDEX interview_scheduling_due_idx ON interview_scheduling_threads(next_at, id)
    WHERE status IN ('new', 'waiting', 'calendar');

CREATE TABLE interview_scheduling_activity (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id        bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    thread_id      bigint NOT NULL REFERENCES interview_scheduling_threads(id) ON DELETE CASCADE,
    kind           text NOT NULL CHECK (kind IN ('reply', 'booking', 'review')),
    happened_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX interview_scheduling_activity_user_idx ON interview_scheduling_activity(user_id, happened_at);
CREATE INDEX interview_scheduling_activity_thread_idx ON interview_scheduling_activity(thread_id);

ALTER TABLE inbox_digest_deliveries
    ADD COLUMN scheduling_activity_ids bigint[] NOT NULL DEFAULT '{}';
