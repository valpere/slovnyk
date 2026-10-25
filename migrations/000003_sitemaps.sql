-- Optional discovery inventory: files must be durable before this transaction.
CREATE TABLE slovnyk.sitemap_inventory (
    dataset_id uuid PRIMARY KEY REFERENCES slovnyk.datasets(id),
    report_sha bytea NOT NULL CHECK(octet_length(report_sha)=32),
    report_bytes bigint NOT NULL CHECK(report_bytes>0),
    index_sha bytea NOT NULL CHECK(octet_length(index_sha)=32),
    index_bytes bigint NOT NULL CHECK(index_bytes BETWEEN 1 AND 52428800),
    url_count bigint NOT NULL CHECK(url_count BETWEEN 1 AND 2500000000),
    shard_count integer NOT NULL CHECK(shard_count BETWEEN 1 AND 50000)
);
CREATE TABLE slovnyk.sitemap_shards (
    dataset_id uuid NOT NULL REFERENCES slovnyk.sitemap_inventory(dataset_id),
    sha256 bytea NOT NULL CHECK(octet_length(sha256)=32),
    byte_size bigint NOT NULL CHECK(byte_size BETWEEN 1 AND 52428800),
    url_count integer NOT NULL CHECK(url_count BETWEEN 1 AND 50000),
    PRIMARY KEY(dataset_id,sha256)
);
CREATE FUNCTION slovnyk.guard_sitemap_inventory() RETURNS trigger LANGUAGE plpgsql
SET search_path=pg_catalog,slovnyk AS $$
BEGIN
    PERFORM 1 FROM slovnyk.datasets WHERE id=NEW.dataset_id AND state='ready' FOR SHARE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='55000', MESSAGE='sitemap requires ready dataset'; END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER sitemap_ready BEFORE INSERT ON slovnyk.sitemap_inventory
FOR EACH ROW EXECUTE FUNCTION slovnyk.guard_sitemap_inventory();
-- Deferred checks prevent partial inventories, even on an accidental partial commit.
CREATE FUNCTION slovnyk.check_sitemap_inventory() RETURNS trigger LANGUAGE plpgsql
SET search_path=pg_catalog,slovnyk AS $$
DECLARE expected slovnyk.sitemap_inventory%ROWTYPE; actual_count bigint; actual_urls bigint;
BEGIN
    SELECT * INTO STRICT expected FROM slovnyk.sitemap_inventory WHERE dataset_id=NEW.dataset_id;
    SELECT count(*),coalesce(sum(url_count),0) INTO actual_count,actual_urls
        FROM slovnyk.sitemap_shards WHERE dataset_id=NEW.dataset_id;
    IF actual_count<>expected.shard_count OR actual_urls<>expected.url_count THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='incomplete sitemap inventory';
    END IF;
    RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER sitemap_complete AFTER INSERT ON slovnyk.sitemap_inventory
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION slovnyk.check_sitemap_inventory();
CREATE TRIGGER sitemap_inventory_immutable BEFORE UPDATE OR DELETE ON slovnyk.sitemap_inventory
FOR EACH ROW EXECUTE FUNCTION slovnyk.guard_event_mutation();
CREATE TRIGGER sitemap_shards_immutable BEFORE UPDATE OR DELETE ON slovnyk.sitemap_shards
FOR EACH ROW EXECUTE FUNCTION slovnyk.guard_event_mutation();
REVOKE ALL ON slovnyk.sitemap_inventory,slovnyk.sitemap_shards FROM PUBLIC;
REVOKE ALL ON FUNCTION slovnyk.guard_sitemap_inventory(),slovnyk.check_sitemap_inventory() FROM PUBLIC;
GRANT SELECT ON slovnyk.sitemap_inventory,slovnyk.sitemap_shards TO slovnyk_http;
GRANT SELECT ON slovnyk.sitemap_inventory,slovnyk.sitemap_shards TO slovnyk_operator;
-- A narrow write API makes the complete inventory append-only without persisted
-- transaction identifiers or per-shard deferred scans. It also supports retry.
CREATE FUNCTION slovnyk.register_sitemap(uuid,bytea,bigint,bytea,bigint,bigint,jsonb)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,slovnyk AS $$
DECLARE existing bytea;
BEGIN
    PERFORM 1 FROM slovnyk.datasets WHERE id=$1 AND state='ready' FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='55000', MESSAGE='sitemap requires ready dataset'; END IF;
    SELECT report_sha INTO existing FROM slovnyk.sitemap_inventory WHERE dataset_id=$1;
    IF FOUND THEN
        IF existing<>$2 THEN RAISE EXCEPTION USING ERRCODE='55000', MESSAGE='sitemap inventory conflict'; END IF;
        RETURN;
    END IF;
    INSERT INTO slovnyk.sitemap_inventory(dataset_id,report_sha,report_bytes,index_sha,index_bytes,url_count,shard_count)
        VALUES($1,$2,$3,$4,$5,$6,jsonb_array_length($7));
    INSERT INTO slovnyk.sitemap_shards(dataset_id,sha256,byte_size,url_count)
        SELECT $1,decode(s->'artifact'->>'sha256','hex'),(s->'artifact'->>'bytes')::bigint,(s->>'urls')::integer
        FROM jsonb_array_elements($7) s;
END $$;
REVOKE ALL ON FUNCTION slovnyk.register_sitemap(uuid,bytea,bigint,bytea,bigint,bigint,jsonb) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION slovnyk.register_sitemap(uuid,bytea,bigint,bytea,bigint,bigint,jsonb) TO slovnyk_operator;
