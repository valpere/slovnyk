-- Identity uses raw UTF-8 bytes, independent of dataset, normalization and UI.
-- convert_to has fixed UTF8 arguments; Migrate already requires a UTF8 database.
CREATE FUNCTION slovnyk.entry_id(text,text,text) RETURNS bytea
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
SET search_path=pg_catalog
AS $$ SELECT sha256(convert_to('slovnyk-entry-v1','UTF8') || decode('00','hex') ||
    int8send(octet_length($1)::bigint) || convert_to($1,'UTF8') ||
    int8send(octet_length($2)::bigint) || convert_to($2,'UTF8') ||
    int8send(octet_length($3)::bigint) || convert_to($3,'UTF8')) $$;
REVOKE ALL ON FUNCTION slovnyk.entry_id(text,text,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION slovnyk.entry_id(text,text,text) TO slovnyk_http,slovnyk_operator;
-- Fixed-size keys admit 64 KiB raw values without B-tree tuple overflow.
CREATE INDEX translations_entry ON slovnyk.translations
    (dataset_id,slovnyk.entry_id(source_locale,target_locale,source_text),id);
