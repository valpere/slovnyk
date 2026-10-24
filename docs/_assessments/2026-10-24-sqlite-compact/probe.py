"""Lossless local physical-key experiment; not a production importer."""

import argparse
import itertools
import json
import resource
import sqlite3
import time
from pathlib import Path


def equal_rows(left, right, label):
    sentinel = object()
    count = 0
    for count, (a, b) in enumerate(itertools.zip_longest(left, right, fillvalue=sentinel), 1):
        if a != b:
            raise ValueError(f'Logical row mismatch: {label}, row {count}')
    return count


def connect(path, readonly=False):
    uri = path.resolve().as_uri() + ('?mode=ro&immutable=1' if readonly else '?mode=rw')
    conn = sqlite3.connect(uri, uri=True)
    conn.execute('PRAGMA cache_size=-8192')
    conn.execute('PRAGMA mmap_size=0')
    conn.execute('PRAGMA temp_store=FILE')
    return conn


def verify(old, new):
    original = old.execute('SELECT * FROM datasets').fetchall()
    compact = new.execute('SELECT uuid,fingerprint,manifest,schema_version,parser_version,mapping_version,normalization_version,state,created_at FROM datasets').fetchall()
    if original != compact or len(original) != 1:
        raise ValueError('Dataset identity/manifest mismatch')
    dataset = original[0][0]
    counts = {}
    columns = 'id,source_locale,target_locale,source_text,target_text,source_key,key_hash,tuple_hash'
    counts['translations'] = equal_rows(
        old.execute(f'SELECT {columns} FROM translations WHERE dataset_id=? ORDER BY id', (dataset,)),
        new.execute(f'SELECT {columns} FROM translations WHERE dataset_id=1 ORDER BY id'), 'translations')
    counts.update(import_records=0, translation_origins=0)
    files = new.execute('SELECT artifact_id,artifact_hash FROM source_files WHERE dataset_id=1 ORDER BY artifact_hash').fetchall()
    if len(files) != old.execute('SELECT COUNT(*) FROM source_files').fetchone()[0]:
        raise ValueError('Source file count mismatch')
    for artifact_id, artifact_hash in files:
        for table, columns in [
            ('import_records', 'ordinal,outcome,translation_id,issue_code'),
            ('translation_origins', 'ordinal,translation_id,source_locale,target_locale,source_text,target_text,legacy_count,legacy_status'),
        ]:
            counts[table] += equal_rows(
                old.execute(f'SELECT {columns} FROM {table} WHERE dataset_id=? AND artifact_hash=? ORDER BY ordinal', (dataset, artifact_hash)),
                new.execute(f'SELECT {columns} FROM {table} WHERE dataset_id=1 AND artifact_id=? ORDER BY ordinal', (artifact_id,)),
                table)
    metadata = {
        'dataset_locales': ('code,display_name', 'code'),
        'dataset_pairs': ('source_locale,target_locale,unique_count', 'source_locale,target_locale'),
        'source_files': ('artifact_hash,relative_name,format,byte_size,complete,checksum_verified,first_translation_id,last_translation_id', 'artifact_hash'),
        'import_runs': ('id,attempt,checkpoint,state,started_at', 'id'),
        'exports': ('export_id,source_locale,target_locale,artifact_reference,sha256,byte_size,logical_rows,verified', 'export_id'),
    }
    for table, (columns, order) in metadata.items():
        counts[table] = equal_rows(
            old.execute(f'SELECT {columns} FROM {table} WHERE dataset_id=? ORDER BY {order}', (dataset,)),
            new.execute(f'SELECT {columns} FROM {table} WHERE dataset_id=1 ORDER BY {order}'), table)
    for table in ['import_issues','legacy_query_snapshots','publication_events']:
        if old.execute(f'SELECT COUNT(*) FROM {table}').fetchone()[0] or new.execute(f'SELECT COUNT(*) FROM {table}').fetchone()[0]:
            raise ValueError(f'This experiment requires empty {table}')
        counts[table] = 0
    if old.execute('SELECT dataset_id,generation FROM active_dataset').fetchall() != [(None, 0)]:
        raise ValueError('Original dataset must not be published')
    if new.execute('SELECT dataset_id,generation FROM active_dataset').fetchall() != [(None, 0)]:
        raise ValueError('Compact dataset was published unexpectedly')
    return counts


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source', type=Path)
    parser.add_argument('database', type=Path)
    parser.add_argument('output', type=Path)
    args = parser.parse_args()
    result = {'complete': False, 'phases_s': {}}
    started = time.monotonic()
    args.database.parent.mkdir(parents=True, exist_ok=True)
    with args.output.open('x') as output:
        try:
            with args.database.open('xb'):
                pass
            old = connect(args.source, readonly=True)
            datasets = old.execute('SELECT id,state FROM datasets').fetchall()
            if len(datasets) != 1 or datasets[0][1] != 'validating':
                raise ValueError('Expected one validating source dataset')
            for table in ['import_issues','legacy_query_snapshots','publication_events']:
                if old.execute(f'SELECT COUNT(*) FROM {table}').fetchone()[0]:
                    raise ValueError(f'Unsupported nonempty {table}')
            if old.execute('SELECT dataset_id,generation FROM active_dataset').fetchall() != [(None, 0)]:
                raise ValueError('Source must remain unpublished')
            new = connect(args.database)
            new.execute('PRAGMA foreign_keys=OFF')
            new.executescript(Path(__file__).with_name('schema.sql').read_text())
            new.execute('ATTACH DATABASE ? AS old', (args.source.resolve().as_uri()+'?mode=ro&immutable=1',))
            phase = time.monotonic()
            new.execute('INSERT INTO datasets SELECT 1,id,fingerprint,manifest,schema_version,parser_version,mapping_version,normalization_version,state,created_at FROM old.datasets')
            new.execute('INSERT INTO dataset_locales SELECT 1,code,display_name FROM old.dataset_locales')
            new.execute('INSERT INTO dataset_pairs SELECT 1,source_locale,target_locale,unique_count FROM old.dataset_pairs')
            new.execute('INSERT INTO source_files SELECT ROW_NUMBER() OVER (ORDER BY artifact_hash),1,artifact_hash,relative_name,format,byte_size,complete,checksum_verified,first_translation_id,last_translation_id FROM old.source_files')
            new.execute('INSERT INTO import_runs SELECT id,1,attempt,checkpoint,state,started_at FROM old.import_runs')
            new.commit()
            print('Copying translations with compact dataset key', flush=True)
            new.execute('INSERT INTO translations SELECT 1,id,source_locale,target_locale,source_text,target_text,source_key,key_hash,tuple_hash FROM old.translations WHERE dataset_id=? ORDER BY id', (datasets[0][0],))
            new.commit()
            files = new.execute('SELECT artifact_id,artifact_hash FROM source_files ORDER BY artifact_id').fetchall()
            for index, (artifact_id, artifact_hash) in enumerate(files, 1):
                new.execute('INSERT INTO import_records SELECT 1,?,ordinal,outcome,translation_id,issue_code FROM old.import_records WHERE dataset_id=? AND artifact_hash=? ORDER BY ordinal', (artifact_id,datasets[0][0],artifact_hash))
                new.execute('INSERT INTO translation_origins SELECT 1,?,ordinal,translation_id,source_locale,target_locale,source_text,target_text,legacy_count,legacy_status FROM old.translation_origins WHERE dataset_id=? AND artifact_hash=? ORDER BY ordinal', (artifact_id,datasets[0][0],artifact_hash))
                new.commit()
                if index % 100 == 0:
                    print(f'Copied ledger/origins {index}/{len(files)} artifacts', flush=True)
            new.execute('INSERT INTO exports SELECT 1,export_id,source_locale,target_locale,artifact_reference,sha256,byte_size,logical_rows,verified FROM old.exports')
            new.commit()
            result['phases_s']['conversion'] = round(time.monotonic()-phase, 3)
            result['bytes_before_indexes'] = args.database.stat().st_size
            print('Building tuple/exact/covering indexes', flush=True)
            phase = time.monotonic()
            new.execute('CREATE UNIQUE INDEX tuple_identity ON translations(dataset_id,tuple_hash)')
            new.execute('CREATE INDEX exact_lookup ON translations(dataset_id,source_locale,target_locale,key_hash)')
            new.execute('CREATE INDEX lookup_order ON translations(dataset_id,source_locale,target_locale,source_key COLLATE BINARY,source_text COLLATE BINARY,target_text COLLATE BINARY,id)')
            new.execute('ANALYZE main')
            new.commit()
            result['phases_s']['indexes_and_analyze'] = round(time.monotonic()-phase, 3)
            print('Checking foreign keys and comparing every logical row', flush=True)
            phase = time.monotonic()
            if new.execute('PRAGMA foreign_key_check').fetchmany(1):
                raise ValueError('Foreign key violations')
            result['foreign_key_check'] = 'ok'
            result['phases_s']['foreign_key_check'] = round(time.monotonic()-phase, 3)
            new.execute('PRAGMA foreign_keys=ON')
            phase = time.monotonic()
            result['compared_rows'] = verify(old, new)
            result['phases_s']['logical_equality'] = round(time.monotonic()-phase, 3)
            result.update(complete=True, bytes_before=args.source.stat().st_size,
                          bytes_after=args.database.stat().st_size,
                          total_s=round(time.monotonic()-started, 3),
                          peak_process_rss_KiB_linux=resource.getrusage(resource.RUSAGE_SELF).ru_maxrss,
                          dataset_state='validating', active_dataset=None, generation=0)
            old.close()
            new.close()
        except Exception as exc:
            result['error'] = f'{type(exc).__name__}: {exc}'
            raise
        finally:
            json.dump(result, output, indent=2)
            output.write('\n')
    print(json.dumps(result))


if __name__ == '__main__':
    main()
