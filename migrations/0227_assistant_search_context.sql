-- Apply before deploying the assistant readers: every session read includes these columns.
ALTER TABLE assistant_sessions
    ADD COLUMN search_query text,
    ADD COLUMN selection_brief text NOT NULL DEFAULT '';

ALTER TABLE assistant_sessions ADD CONSTRAINT assistant_search_context_chat_only
    CHECK (search_query IS NULL OR preset = 'chat') NOT VALID;

ALTER TABLE assistant_sessions ADD CONSTRAINT assistant_search_context_bounds
    CHECK ((search_query IS NULL OR octet_length(search_query) <= 8000)
        AND char_length(selection_brief) <= 2000
        AND (search_query IS NOT NULL OR selection_brief = '')) NOT VALID;
