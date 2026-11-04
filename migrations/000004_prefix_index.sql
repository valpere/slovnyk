-- Owner-run maintenance operation: block writers, retain ordinary readers.
-- Keep the migration bounded while allowing a full-corpus index build.
SET LOCAL statement_timeout='30min';
CREATE INDEX translations_prefix64 ON slovnyk.translations
 (dataset_id,source_locale,target_locale,(left(source_key,64) COLLATE "C"));
SET LOCAL statement_timeout='30s';
