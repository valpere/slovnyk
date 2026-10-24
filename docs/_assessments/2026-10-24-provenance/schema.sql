-- Experimental SQLite representation of DATA_MODEL, not a production migration.
-- One dataset is loaded; logical composite dataset keys are retained explicitly.
CREATE TABLE datasets (
    id BLOB PRIMARY KEY CHECK(length(id)=16),
    fingerprint BLOB NOT NULL UNIQUE CHECK(length(fingerprint)=32),
    manifest BLOB NOT NULL,
    schema_version TEXT NOT NULL,
    parser_version TEXT NOT NULL,
    mapping_version TEXT NOT NULL,
    normalization_version TEXT NOT NULL,
    state TEXT NOT NULL CHECK(state IN ('loading','validating','ready','failed')),
    created_at TEXT NOT NULL
) WITHOUT ROWID;
CREATE TABLE active_dataset (
    singleton INTEGER PRIMARY KEY CHECK(singleton=1),
    dataset_id BLOB REFERENCES datasets(id),
    generation INTEGER NOT NULL CHECK(generation>=0)
);
INSERT INTO active_dataset VALUES(1,NULL,0);
CREATE TABLE dataset_locales (
    dataset_id BLOB NOT NULL REFERENCES datasets(id),
    code TEXT NOT NULL CHECK(length(CAST(code AS BLOB)) BETWEEN 1 AND 64),
    display_name TEXT,
    PRIMARY KEY(dataset_id,code)
) WITHOUT ROWID;
CREATE TABLE dataset_pairs (
    dataset_id BLOB NOT NULL,
    source_locale TEXT NOT NULL,
    target_locale TEXT NOT NULL,
    unique_count INTEGER NOT NULL CHECK(unique_count>=0),
    PRIMARY KEY(dataset_id,source_locale,target_locale),
    FOREIGN KEY(dataset_id,source_locale) REFERENCES dataset_locales(dataset_id,code),
    FOREIGN KEY(dataset_id,target_locale) REFERENCES dataset_locales(dataset_id,code)
) WITHOUT ROWID;
CREATE TABLE source_files (
    dataset_id BLOB NOT NULL REFERENCES datasets(id),
    artifact_hash BLOB NOT NULL CHECK(length(artifact_hash)=32),
    relative_name TEXT NOT NULL,
    format TEXT NOT NULL,
    byte_size INTEGER NOT NULL CHECK(byte_size>=0),
    complete INTEGER NOT NULL CHECK(complete IN (0,1)),
    checksum_verified INTEGER NOT NULL CHECK(checksum_verified IN (0,1)),
    first_translation_id INTEGER NOT NULL,
    last_translation_id INTEGER NOT NULL,
    PRIMARY KEY(dataset_id,artifact_hash)
) WITHOUT ROWID;
CREATE TABLE translations (
    dataset_id BLOB NOT NULL,
    id INTEGER NOT NULL CHECK(id>0),
    source_locale TEXT NOT NULL,
    target_locale TEXT NOT NULL,
    source_text TEXT NOT NULL CHECK(length(CAST(source_text AS BLOB)) BETWEEN 1 AND 65536),
    target_text TEXT NOT NULL CHECK(length(CAST(target_text AS BLOB)) BETWEEN 1 AND 65536),
    source_key TEXT NOT NULL,
    key_hash BLOB NOT NULL CHECK(length(key_hash)=32),
    tuple_hash BLOB NOT NULL CHECK(length(tuple_hash)=32),
    PRIMARY KEY(dataset_id,id),
    FOREIGN KEY(dataset_id,source_locale,target_locale)
        REFERENCES dataset_pairs(dataset_id,source_locale,target_locale)
) WITHOUT ROWID;
-- The unique tuple index is installed after bulk loading, before validation.
CREATE TABLE import_records (
    dataset_id BLOB NOT NULL,
    artifact_hash BLOB NOT NULL,
    ordinal INTEGER NOT NULL CHECK(ordinal>0),
    outcome TEXT NOT NULL CHECK(outcome IN ('unique','duplicate','rejected')),
    translation_id INTEGER,
    issue_code TEXT,
    PRIMARY KEY(dataset_id,artifact_hash,ordinal),
    FOREIGN KEY(dataset_id,artifact_hash) REFERENCES source_files(dataset_id,artifact_hash),
    FOREIGN KEY(dataset_id,translation_id) REFERENCES translations(dataset_id,id)
) WITHOUT ROWID;
CREATE TABLE translation_origins (
    dataset_id BLOB NOT NULL,
    artifact_hash BLOB NOT NULL,
    ordinal INTEGER NOT NULL CHECK(ordinal>0),
    translation_id INTEGER NOT NULL,
    source_locale TEXT NOT NULL,
    target_locale TEXT NOT NULL,
    source_text TEXT NOT NULL,
    target_text TEXT NOT NULL,
    legacy_count INTEGER,
    legacy_status INTEGER,
    PRIMARY KEY(dataset_id,artifact_hash,ordinal),
    FOREIGN KEY(dataset_id,artifact_hash,ordinal)
        REFERENCES import_records(dataset_id,artifact_hash,ordinal),
    FOREIGN KEY(dataset_id,translation_id) REFERENCES translations(dataset_id,id)
) WITHOUT ROWID;
CREATE TABLE import_runs (
    id BLOB PRIMARY KEY CHECK(length(id)=16),
    dataset_id BLOB NOT NULL REFERENCES datasets(id),
    attempt INTEGER NOT NULL,
    checkpoint TEXT NOT NULL,
    state TEXT NOT NULL,
    started_at TEXT NOT NULL
) WITHOUT ROWID;
CREATE TABLE import_issues (
    id INTEGER PRIMARY KEY,
    dataset_id BLOB NOT NULL REFERENCES datasets(id),
    artifact_hash BLOB,
    ordinal INTEGER,
    byte_offset INTEGER,
    reason_code TEXT NOT NULL,
    quarantine_reference TEXT
);
CREATE TABLE legacy_query_snapshots (
    id INTEGER PRIMARY KEY,
    dataset_id BLOB NOT NULL,
    artifact_hash BLOB NOT NULL,
    ordinal INTEGER NOT NULL,
    input_locale TEXT NOT NULL,
    output_locale TEXT NOT NULL,
    word TEXT NOT NULL,
    legacy_count INTEGER,
    raw_modified_timestamp TEXT,
    timezone_known INTEGER NOT NULL CHECK(timezone_known IN (0,1)),
    FOREIGN KEY(dataset_id,artifact_hash) REFERENCES source_files(dataset_id,artifact_hash)
);
CREATE TABLE publication_events (
    generation INTEGER PRIMARY KEY,
    previous_dataset_id BLOB REFERENCES datasets(id),
    new_dataset_id BLOB NOT NULL REFERENCES datasets(id),
    operator_reference TEXT NOT NULL,
    published_at TEXT NOT NULL
);
CREATE TABLE exports (
    dataset_id BLOB NOT NULL,
    export_id BLOB NOT NULL CHECK(length(export_id)=16),
    source_locale TEXT NOT NULL,
    target_locale TEXT NOT NULL,
    artifact_reference TEXT NOT NULL,
    sha256 BLOB NOT NULL CHECK(length(sha256)=32),
    byte_size INTEGER NOT NULL,
    logical_rows INTEGER NOT NULL,
    verified INTEGER NOT NULL CHECK(verified IN (0,1)),
    PRIMARY KEY(dataset_id,export_id),
    FOREIGN KEY(dataset_id,source_locale,target_locale)
        REFERENCES dataset_pairs(dataset_id,source_locale,target_locale)
) WITHOUT ROWID;
