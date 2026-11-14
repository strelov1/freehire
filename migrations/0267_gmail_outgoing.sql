-- Apply before deploying binaries that read message direction and sent cursors.
ALTER TABLE emails
 ADD COLUMN direction text NOT NULL DEFAULT 'incoming' CHECK (direction IN ('incoming', 'outgoing')),
 ADD COLUMN to_addrs text NOT NULL DEFAULT '',
 ADD COLUMN cc_addrs text NOT NULL DEFAULT '';
ALTER TABLE gmail_connections ADD COLUMN sent_sync_cursor bigint NOT NULL DEFAULT 0;
