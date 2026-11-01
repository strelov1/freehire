-- Validate the expanded dictionary-gap kind check separately so the migration adding it stays brief.
ALTER TABLE dictionary_gaps
    VALIDATE CONSTRAINT dictionary_gaps_kind_check;
