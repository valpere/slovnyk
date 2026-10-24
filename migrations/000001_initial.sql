-- Runs transactionally as the migration/schema owner, never the HTTP role.
CREATE DOMAIN slovnyk.digest AS bytea CHECK (octet_length(VALUE) = 32);
CREATE DOMAIN slovnyk.locale AS text COLLATE "C"
    CHECK (octet_length(VALUE) BETWEEN 1 AND 64);
CREATE DOMAIN slovnyk.raw_text AS text COLLATE "C"
    CHECK (octet_length(VALUE) BETWEEN 1 AND 65536);

CREATE TABLE slovnyk.datasets (
    id uuid PRIMARY KEY,
    fingerprint slovnyk.digest NOT NULL UNIQUE,
    manifest_bytes bytea NOT NULL,
    schema_version integer NOT NULL CHECK (schema_version = 1),
    parser_version text NOT NULL,
    mapping_version text NOT NULL,
    normalization_version text NOT NULL,
    state text NOT NULL DEFAULT 'created' CHECK (state IN ('created','loading','validating','ready','failed')),
    report_path text,
    report_hash slovnyk.digest,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CHECK (fingerprint = sha256(manifest_bytes)),
    CHECK ((report_path IS NULL) = (report_hash IS NULL)),
    CHECK (state <> 'ready' OR report_path IS NOT NULL)
);
CREATE TABLE slovnyk.dataset_locales (
    dataset_id uuid NOT NULL REFERENCES slovnyk.datasets(id),
    code slovnyk.locale NOT NULL,
    display_name text,
    PRIMARY KEY (dataset_id,code)
);
CREATE TABLE slovnyk.dataset_pairs (
    dataset_id uuid NOT NULL,
    source_locale slovnyk.locale NOT NULL,
    target_locale slovnyk.locale NOT NULL,
    unique_count bigint NOT NULL DEFAULT 0 CHECK (unique_count >= 0),
    PRIMARY KEY (dataset_id,source_locale,target_locale),
    FOREIGN KEY (dataset_id,source_locale) REFERENCES slovnyk.dataset_locales(dataset_id,code),
    FOREIGN KEY (dataset_id,target_locale) REFERENCES slovnyk.dataset_locales(dataset_id,code)
);
CREATE TABLE slovnyk.translations (
    dataset_id uuid NOT NULL,
    id bigint NOT NULL CHECK (id > 0),
    source_locale slovnyk.locale NOT NULL,
    target_locale slovnyk.locale NOT NULL,
    source_text slovnyk.raw_text NOT NULL,
    target_text slovnyk.raw_text NOT NULL,
    source_key text COLLATE "C" NOT NULL CHECK (octet_length(source_key) > 0),
    key_hash slovnyk.digest NOT NULL,
    tuple_hash slovnyk.digest NOT NULL,
    PRIMARY KEY (dataset_id,id),
    UNIQUE (dataset_id,tuple_hash),
    FOREIGN KEY (dataset_id,source_locale,target_locale)
        REFERENCES slovnyk.dataset_pairs(dataset_id,source_locale,target_locale),
    CHECK (key_hash = sha256(convert_to(source_key,'UTF8'))),
    CHECK (tuple_hash = sha256(
        int8send(octet_length(source_locale)::bigint) || convert_to(source_locale,'UTF8') ||
        int8send(octet_length(target_locale)::bigint) || convert_to(target_locale,'UTF8') ||
        int8send(octet_length(source_text)::bigint) || convert_to(source_text,'UTF8') ||
        int8send(octet_length(target_text)::bigint) || convert_to(target_text,'UTF8')))
);
CREATE INDEX translations_exact ON slovnyk.translations(dataset_id,source_locale,target_locale,key_hash);
-- No B-tree index over raw texts or expanded source_key: admitted values exceed index tuple limits.
CREATE TABLE slovnyk.source_files (
    dataset_id uuid NOT NULL REFERENCES slovnyk.datasets(id),
    artifact_hash slovnyk.digest NOT NULL,
    relative_name text NOT NULL,
    format text NOT NULL CHECK (format = 'csv4.gz'),
    byte_size bigint NOT NULL CHECK (byte_size > 0),
    complete boolean NOT NULL DEFAULT false,
    checksum_verified boolean NOT NULL DEFAULT false,
    parsed bigint NOT NULL DEFAULT 0 CHECK (parsed >= 0),
    accepted_unique bigint NOT NULL DEFAULT 0 CHECK (accepted_unique >= 0),
    duplicate_occurrences bigint NOT NULL DEFAULT 0 CHECK (duplicate_occurrences >= 0),
    rejected bigint NOT NULL DEFAULT 0 CHECK (rejected >= 0),
    PRIMARY KEY (dataset_id,artifact_hash),
    UNIQUE (dataset_id,relative_name),
    CHECK (NOT complete OR (checksum_verified AND parsed::numeric = accepted_unique::numeric + duplicate_occurrences::numeric + rejected::numeric))
);
CREATE TABLE slovnyk.import_runs (
    id uuid PRIMARY KEY,
    dataset_id uuid NOT NULL REFERENCES slovnyk.datasets(id),
    attempt bigint NOT NULL CHECK (attempt > 0),
    state text NOT NULL CHECK (state IN ('running','interrupted','completed','failed')),
    error_class text,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE (dataset_id,id),
    UNIQUE (dataset_id,attempt)
);
CREATE UNIQUE INDEX import_one_running ON slovnyk.import_runs(dataset_id) WHERE state = 'running';
CREATE TABLE slovnyk.import_records (
    dataset_id uuid NOT NULL,
    artifact_hash slovnyk.digest NOT NULL,
    ordinal bigint NOT NULL CHECK (ordinal > 0),
    outcome text NOT NULL CHECK (outcome IN ('unique','duplicate','rejected')),
    translation_id bigint,
    issue_code text,
    PRIMARY KEY (dataset_id,artifact_hash,ordinal),
    UNIQUE (dataset_id,artifact_hash,ordinal,translation_id),
    FOREIGN KEY (dataset_id,artifact_hash) REFERENCES slovnyk.source_files(dataset_id,artifact_hash),
    FOREIGN KEY (dataset_id,translation_id) REFERENCES slovnyk.translations(dataset_id,id),
    CHECK ((outcome IN ('unique','duplicate') AND translation_id IS NOT NULL AND issue_code IS NULL)
        OR (outcome = 'rejected' AND translation_id IS NULL AND issue_code IS NOT NULL))
);
CREATE TABLE slovnyk.translation_origins (
    dataset_id uuid NOT NULL,
    artifact_hash slovnyk.digest NOT NULL,
    ordinal bigint NOT NULL,
    translation_id bigint NOT NULL,
    source_locale slovnyk.locale NOT NULL,
    target_locale slovnyk.locale NOT NULL,
    source_text slovnyk.raw_text NOT NULL,
    target_text slovnyk.raw_text NOT NULL,
    legacy_count bigint,
    legacy_status bigint,
    source_locator text NOT NULL,
    provenance jsonb NOT NULL DEFAULT '{}',
    PRIMARY KEY (dataset_id,artifact_hash,ordinal),
    FOREIGN KEY (dataset_id,artifact_hash,ordinal,translation_id)
        REFERENCES slovnyk.import_records(dataset_id,artifact_hash,ordinal,translation_id)
);
CREATE TABLE slovnyk.import_checkpoints (
    dataset_id uuid NOT NULL,
    artifact_hash slovnyk.digest NOT NULL,
    run_id uuid NOT NULL,
    committed_ordinal bigint NOT NULL CHECK (committed_ordinal >= 0),
    PRIMARY KEY (dataset_id,artifact_hash),
    FOREIGN KEY (dataset_id,artifact_hash) REFERENCES slovnyk.source_files(dataset_id,artifact_hash),
    FOREIGN KEY (dataset_id,run_id) REFERENCES slovnyk.import_runs(dataset_id,id)
);
CREATE TABLE slovnyk.import_issues (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    dataset_id uuid NOT NULL,
    artifact_hash slovnyk.digest NOT NULL,
    ordinal bigint CHECK (ordinal > 0),
    byte_offset bigint CHECK (byte_offset >= 0),
    reason_code text NOT NULL,
    quarantine_ref text,
    FOREIGN KEY (dataset_id,artifact_hash) REFERENCES slovnyk.source_files(dataset_id,artifact_hash),
    CHECK (ordinal IS NOT NULL OR byte_offset IS NOT NULL)
);
CREATE TABLE slovnyk.legacy_query_snapshots (
    dataset_id uuid NOT NULL,
    artifact_hash slovnyk.digest NOT NULL,
    ordinal bigint NOT NULL CHECK (ordinal > 0),
    input_locale slovnyk.locale NOT NULL,
    output_locale slovnyk.locale NOT NULL,
    word slovnyk.raw_text NOT NULL,
    legacy_count bigint,
    modified_raw text,
    source_timestamp_type text,
    timezone_status text NOT NULL CHECK (timezone_status IN ('known','unknown')),
    PRIMARY KEY (dataset_id,artifact_hash,ordinal),
    FOREIGN KEY (dataset_id,artifact_hash) REFERENCES slovnyk.source_files(dataset_id,artifact_hash),
    FOREIGN KEY (dataset_id,input_locale) REFERENCES slovnyk.dataset_locales(dataset_id,code),
    FOREIGN KEY (dataset_id,output_locale) REFERENCES slovnyk.dataset_locales(dataset_id,code)
);
CREATE TABLE slovnyk.exports (
    dataset_id uuid NOT NULL,
    export_id uuid NOT NULL,
    source_locale slovnyk.locale NOT NULL,
    target_locale slovnyk.locale NOT NULL,
    artifact_path text NOT NULL,
    sha256 slovnyk.digest NOT NULL,
    byte_size bigint NOT NULL CHECK (byte_size > 0),
    logical_rows bigint NOT NULL CHECK (logical_rows >= 0),
    verified boolean NOT NULL DEFAULT false,
    PRIMARY KEY (dataset_id,export_id),
    UNIQUE (artifact_path),
    FOREIGN KEY (dataset_id,source_locale,target_locale)
        REFERENCES slovnyk.dataset_pairs(dataset_id,source_locale,target_locale)
);
CREATE TABLE slovnyk.active_dataset (
    singleton integer PRIMARY KEY CHECK (singleton = 1),
    dataset_id uuid REFERENCES slovnyk.datasets(id),
    generation bigint NOT NULL CHECK (generation >= 0),
    CHECK ((generation = 0 AND dataset_id IS NULL) OR (generation > 0 AND dataset_id IS NOT NULL))
);
INSERT INTO slovnyk.active_dataset VALUES (1,NULL,0);
CREATE TABLE slovnyk.publication_events (
    generation bigint PRIMARY KEY CHECK (generation > 0),
    previous_dataset_id uuid REFERENCES slovnyk.datasets(id),
    new_dataset_id uuid NOT NULL REFERENCES slovnyk.datasets(id),
    operator_ref text NOT NULL,
    run_id uuid,
    published_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY (new_dataset_id,run_id) REFERENCES slovnyk.import_runs(dataset_id,id)
);

CREATE FUNCTION slovnyk.guard_dataset() RETURNS trigger LANGUAGE plpgsql
SET search_path = pg_catalog, slovnyk AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION USING ERRCODE='55000', MESSAGE='dataset deletion disabled';
    END IF;
    IF TG_OP = 'INSERT' THEN
        IF NEW.state <> 'created' THEN
            RAISE EXCEPTION USING ERRCODE='55000', MESSAGE='dataset must start created';
        END IF;
        RETURN NEW;
    END IF;
    IF OLD.state IN ('ready','failed') THEN
        RAISE EXCEPTION USING ERRCODE='55000', MESSAGE='terminal dataset immutable';
    END IF;
    IF (NEW.id,NEW.fingerprint,NEW.manifest_bytes,NEW.schema_version,NEW.parser_version,
        NEW.mapping_version,NEW.normalization_version,NEW.created_at) IS DISTINCT FROM
       (OLD.id,OLD.fingerprint,OLD.manifest_bytes,OLD.schema_version,OLD.parser_version,
        OLD.mapping_version,OLD.normalization_version,OLD.created_at) THEN
        RAISE EXCEPTION USING ERRCODE='55000', MESSAGE='dataset identity immutable';
    END IF;
    IF NEW.state <> OLD.state AND NOT (
        (OLD.state = 'created' AND NEW.state IN ('loading','failed')) OR
        (OLD.state = 'loading' AND NEW.state IN ('validating','failed')) OR
        (OLD.state = 'validating' AND NEW.state IN ('ready','failed'))) THEN
        RAISE EXCEPTION USING ERRCODE='55000', MESSAGE='invalid dataset transition';
    END IF;
    NEW.updated_at = clock_timestamp();
    RETURN NEW;
END $$;
CREATE TRIGGER dataset_guard BEFORE INSERT OR UPDATE OR DELETE ON slovnyk.datasets
FOR EACH ROW EXECUTE FUNCTION slovnyk.guard_dataset();

-- FOR SHARE conflicts with a concurrent ready transition. KEY SHARE does not.
CREATE FUNCTION slovnyk.guard_content() RETURNS trigger LANGUAGE plpgsql
SET search_path = pg_catalog, slovnyk AS $$
DECLARE d uuid; s text;
BEGIN
    IF TG_OP = 'DELETE' THEN d = OLD.dataset_id; ELSE d = NEW.dataset_id; END IF;
    IF TG_OP = 'UPDATE' AND NEW.dataset_id IS DISTINCT FROM OLD.dataset_id THEN
        RAISE EXCEPTION USING ERRCODE='55000', MESSAGE='dataset move forbidden';
    END IF;
    SELECT state INTO s FROM slovnyk.datasets WHERE id=d FOR SHARE;
    IF s IN ('ready','failed') THEN
        RAISE EXCEPTION USING ERRCODE='55000', MESSAGE='terminal content immutable';
    END IF;
    IF TG_OP = 'DELETE' THEN RETURN OLD; ELSE RETURN NEW; END IF;
END $$;
DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['dataset_locales','dataset_pairs','translations','source_files',
        'import_records','translation_origins','import_checkpoints','import_issues','legacy_query_snapshots','exports']
    LOOP
        EXECUTE format('CREATE TRIGGER content_guard BEFORE INSERT OR UPDATE OR DELETE ON slovnyk.%I FOR EACH ROW EXECUTE FUNCTION slovnyk.guard_content()', t);
    END LOOP;
END $$;
CREATE FUNCTION slovnyk.guard_origin() RETURNS trigger LANGUAGE plpgsql
SET search_path = pg_catalog, slovnyk AS $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM slovnyk.translations t WHERE t.dataset_id=NEW.dataset_id AND t.id=NEW.translation_id
        AND (t.source_locale,t.target_locale,t.source_text,t.target_text) =
            (NEW.source_locale,NEW.target_locale,NEW.source_text,NEW.target_text)) THEN
        RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='origin raw identity mismatch';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER origin_guard BEFORE INSERT OR UPDATE ON slovnyk.translation_origins
FOR EACH ROW EXECUTE FUNCTION slovnyk.guard_origin();

-- A provenance origin must not become stale after a staging translation edit.
-- Import inserts/merges occurrences; it never edits the identified raw tuple.
CREATE FUNCTION slovnyk.guard_translation_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION USING ERRCODE='55000', MESSAGE='translation identity immutable'; END $$;
CREATE TRIGGER translation_immutable BEFORE UPDATE ON slovnyk.translations
FOR EACH ROW EXECUTE FUNCTION slovnyk.guard_translation_update();

CREATE FUNCTION slovnyk.guard_pointer() RETURNS trigger LANGUAGE plpgsql
SET search_path = pg_catalog, slovnyk AS $$
BEGIN
    IF NEW.singleton <> OLD.singleton OR NEW.generation <> OLD.generation + 1 OR NEW.dataset_id IS NULL THEN
        RAISE EXCEPTION USING ERRCODE='55000', MESSAGE='invalid publication generation';
    END IF;
    PERFORM 1 FROM slovnyk.datasets WHERE id=NEW.dataset_id AND state='ready' FOR SHARE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='55000', MESSAGE='publication requires ready dataset'; END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER pointer_guard BEFORE UPDATE ON slovnyk.active_dataset
FOR EACH ROW EXECUTE FUNCTION slovnyk.guard_pointer();
CREATE FUNCTION slovnyk.guard_event_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION USING ERRCODE='55000', MESSAGE='publication history append only'; END $$;
CREATE TRIGGER event_immutable BEFORE UPDATE OR DELETE ON slovnyk.publication_events
FOR EACH ROW EXECUTE FUNCTION slovnyk.guard_event_mutation();
CREATE FUNCTION slovnyk.check_publication() RETURNS trigger LANGUAGE plpgsql
SET search_path = pg_catalog, slovnyk AS $$
BEGIN
    IF TG_TABLE_NAME = 'active_dataset' THEN
        IF NOT EXISTS (SELECT 1 FROM slovnyk.publication_events e WHERE e.generation=NEW.generation
            AND e.new_dataset_id=NEW.dataset_id AND e.previous_dataset_id IS NOT DISTINCT FROM OLD.dataset_id) THEN
            RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='publication event required';
        END IF;
    ELSE
        IF NOT EXISTS (SELECT 1 FROM slovnyk.active_dataset a WHERE a.generation=NEW.generation AND a.dataset_id=NEW.new_dataset_id) THEN
            RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='event must match active pointer';
        END IF;
    END IF;
    RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER pointer_event AFTER UPDATE ON slovnyk.active_dataset
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION slovnyk.check_publication();
CREATE CONSTRAINT TRIGGER event_pointer AFTER INSERT ON slovnyk.publication_events
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION slovnyk.check_publication();

-- Cluster-wide group roles, no credentials. Individual LOGINs are provisioned separately.
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='slovnyk_http') THEN
        CREATE ROLE slovnyk_http NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='slovnyk_operator') THEN
        CREATE ROLE slovnyk_operator NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname IN ('slovnyk_http','slovnyk_operator')
        AND (rolcanlogin OR rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls))
        OR EXISTS (SELECT 1 FROM pg_auth_members m JOIN pg_roles r ON r.oid=m.member
            WHERE r.rolname IN ('slovnyk_http','slovnyk_operator')) THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='unsafe existing application group roles';
    END IF;
END $$;
REVOKE ALL ON SCHEMA slovnyk FROM PUBLIC;
REVOKE ALL ON ALL TABLES IN SCHEMA slovnyk FROM PUBLIC;
REVOKE ALL ON ALL FUNCTIONS IN SCHEMA slovnyk FROM PUBLIC;
GRANT USAGE ON SCHEMA slovnyk TO slovnyk_http, slovnyk_operator;
GRANT SELECT ON slovnyk.schema_migrations, slovnyk.datasets, slovnyk.dataset_locales,
    slovnyk.dataset_pairs, slovnyk.translations, slovnyk.active_dataset,
    slovnyk.publication_events, slovnyk.exports TO slovnyk_http;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA slovnyk TO slovnyk_operator;
REVOKE ALL ON slovnyk.schema_migrations FROM slovnyk_operator;
GRANT SELECT ON slovnyk.schema_migrations TO slovnyk_operator;
REVOKE INSERT, DELETE ON slovnyk.active_dataset FROM slovnyk_operator;
REVOKE UPDATE, DELETE ON slovnyk.publication_events FROM slovnyk_operator;
GRANT USAGE ON ALL SEQUENCES IN SCHEMA slovnyk TO slovnyk_operator;
