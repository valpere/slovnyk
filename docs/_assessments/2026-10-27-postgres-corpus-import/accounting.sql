-- Read-only accounting queries executed 2026-10-27 on the isolated corpus DB.
-- No tuple output. Run after the writer completes; compare JSON with assessment evidence.

SELECT jsonb_object_agg(encode(artifact_hash,'hex'),to_jsonb(x)-'artifact_hash') FROM (SELECT artifact_hash,count(*) n,min(ordinal) first,max(ordinal) last,count(*) FILTER(WHERE outcome='unique') u,count(*) FILTER(WHERE outcome='duplicate') d,count(*) FILTER(WHERE outcome='rejected') r FROM slovnyk.import_records GROUP BY artifact_hash) x;

SELECT jsonb_object_agg(encode(artifact_hash,'hex'),n) FROM (SELECT artifact_hash,count(*) n FROM slovnyk.translation_origins GROUP BY artifact_hash) x;

SELECT jsonb_agg(to_jsonb(x)) FROM (SELECT source_locale,target_locale,count(*) n FROM slovnyk.translations GROUP BY source_locale,target_locale ORDER BY source_locale,target_locale) x;

SELECT jsonb_object_agg(encode(artifact_hash,'hex'),committed_ordinal) FROM slovnyk.import_checkpoints;

SELECT jsonb_agg(to_jsonb(x)) FROM (SELECT source_locale,target_locale,unique_count FROM slovnyk.dataset_pairs ORDER BY source_locale,target_locale) x;

WITH bounds AS (SELECT decode(key,'hex') artifact_hash,value::bigint lim FROM jsonb_each_text('{"000fe9247e4b293db29844e04147c7f8ca6221eafae11828a55a6ab2f9c9b6b6": 55000}'::jsonb)),
 r AS (SELECT r.* FROM slovnyk.import_records r JOIN bounds b USING(artifact_hash) WHERE r.dataset_id='96541c0b-ac71-4e38-b57c-b593e82ac9d5' AND r.ordinal<=b.lim),
 o AS (SELECT o.* FROM slovnyk.translation_origins o JOIN bounds b USING(artifact_hash) WHERE o.dataset_id='96541c0b-ac71-4e38-b57c-b593e82ac9d5' AND o.ordinal<=b.lim),
 t AS (SELECT t.* FROM slovnyk.translations t WHERE t.dataset_id='96541c0b-ac71-4e38-b57c-b593e82ac9d5' AND t.id IN(SELECT translation_id FROM r))
 SELECT jsonb_build_object('bounds','{"000fe9247e4b293db29844e04147c7f8ca6221eafae11828a55a6ab2f9c9b6b6": 55000}'::jsonb,
 'ledger_count',(SELECT count(*) FROM r),'origins_count',(SELECT count(*) FROM o),'translations_count',(SELECT count(*) FROM t),
 'ledger_sha256',(SELECT encode(sha256(convert_to(coalesce(string_agg(encode(sha256(convert_to(to_jsonb(r)::text,'UTF8')),'hex'),'' ORDER BY artifact_hash,ordinal),''),'UTF8')),'hex') FROM r),
 'origins_sha256',(SELECT encode(sha256(convert_to(coalesce(string_agg(encode(sha256(convert_to(to_jsonb(o)::text,'UTF8')),'hex'),'' ORDER BY artifact_hash,ordinal),''),'UTF8')),'hex') FROM o),
 'translations_sha256',(SELECT encode(sha256(convert_to(coalesce(string_agg(encode(sha256(convert_to(to_jsonb(t)::text,'UTF8')),'hex'),'' ORDER BY id),''),'UTF8')),'hex') FROM t));
