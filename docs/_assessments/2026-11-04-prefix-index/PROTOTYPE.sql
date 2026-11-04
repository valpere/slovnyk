-- Experimental standalone translations copy, not a service migration.
-- source_key/source_locale/target_locale/source_text/target_text use COLLATE "C".
CREATE INDEX translations_prefix64 ON translations (
    dataset_id, source_locale, target_locale,
    (left(source_key, 64) COLLATE "C")
);

-- First-page candidate. $5 is an escaped literal prefix plus %, derived from
-- compiled $4 up to its first unescaped wildcard, capped at 64 Unicode runes.
-- Omit the additional predicate entirely if there is no literal prefix.
-- $6 is UI limit + 1, as in ReadPage; seek pagination keeps the full C tuple.
SELECT id, source_key, source_text, target_text
FROM translations
WHERE dataset_id = $1 AND source_locale = $2 AND target_locale = $3
  AND source_key LIKE $4 ESCAPE '\'
  AND left(source_key, 64) COLLATE "C" LIKE $5 ESCAPE '\'
ORDER BY source_key COLLATE "C", source_text COLLATE "C",
         target_text COLLATE "C", id
LIMIT $6;
