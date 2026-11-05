-- What became of the campaign and digest mail, as SES reports it. One row per message and
-- event type, the first time that type is seen: a rate needs "was this mail opened", and a
-- second open adds nothing to it. There is deliberately no user id and no address, so a row
-- says how a letter fared rather than what a person did.
CREATE TABLE email_events (
    message_id  text        NOT NULL,
    type        text        NOT NULL CHECK (type IN ('send', 'delivery', 'open', 'click', 'bounce', 'complaint')),
    campaign    text        NOT NULL,
    variant     text        NOT NULL DEFAULT '',
    occurred_at timestamptz NOT NULL,
    PRIMARY KEY (message_id, type)
);

-- The retention pass walks rows by age.
CREATE INDEX email_events_occurred_at_idx ON email_events (occurred_at);

-- What outlives the 90-day retention of email_events: counts with no message in them.
CREATE TABLE email_event_daily (
    day      date   NOT NULL,
    campaign text   NOT NULL,
    variant  text   NOT NULL,
    type     text   NOT NULL,
    count    bigint NOT NULL,
    PRIMARY KEY (day, campaign, variant, type)
);
