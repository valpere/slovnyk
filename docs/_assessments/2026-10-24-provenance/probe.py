"""Full-corpus SQLite storage/reconciliation experiment; not a production importer.

Populate the logical DATA_MODEL using the existing isolated serving snapshot,
then independently compare every translation and origin to original CSV records.
Dataset stays validating: production publication/durability is not implemented.
"""

import argparse
import csv
import gzip
import hashlib
import io
import json
import os
import platform
import resource
import sqlite3
import struct
import time
import unicodedata
import uuid
from datetime import datetime, timezone
from pathlib import Path


PREFIXES = {}


def tuple_bytes(sl, tl, source, target):
    pair = (sl, tl)
    if pair not in PREFIXES:
        a, b = sl.encode('utf-8'), tl.encode('utf-8')
        PREFIXES[pair] = struct.pack('>Q', len(a)) + a + struct.pack('>Q', len(b)) + b
    a, b = source.encode('utf-8'), target.encode('utf-8')
    return PREFIXES[pair] + struct.pack('>Q', len(a)) + a + struct.pack('>Q', len(b)) + b


def tuple_hash(sl, tl, source, target):
    return hashlib.sha256(tuple_bytes(sl, tl, source, target)).digest()


def normalize(value):
    return unicodedata.normalize('NFC', unicodedata.normalize('NFC', value).casefold())


def file_hash(path):
    digest = hashlib.sha256()
    with path.open('rb') as stream:
        for chunk in iter(lambda: stream.read(8 * 1024 * 1024), b''):
            digest.update(chunk)
    return digest.digest()


def load_schema(conn):
    conn.executescript(Path(__file__).with_name('schema.sql').read_text())


def log(message):
    print(message, flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source', type=Path)
    parser.add_argument('serving_database', type=Path)
    parser.add_argument('audit_result', type=Path)
    parser.add_argument('serving_result', type=Path)
    parser.add_argument('database', type=Path)
    parser.add_argument('exports', type=Path)
    parser.add_argument('output', type=Path)
    args = parser.parse_args()
    started = time.monotonic()
    result = {'complete': False, 'phases_s': {}, 'pairs': [], 'limitations': [
        'SQLite experiment, not a selected engine or production migration',
        'Bulk load uses existing serving snapshot; not gzip importer retry/crash testing',
        'FK enforcement is off only during bulk load; full foreign_key_check required afterwards',
        'Only audited zero-duplicate zero-rejection input supported; no production dedup/quarantine',
        'Source-file id ranges are experimental provenance construction, verified against every CSV ordinal',
        'No production publication, CAS, durability, GC, HTTP or target-server capacity test',
        'Export files remain local experimental artifacts; dataset remains validating',
    ]}
    # Reserve all new artifacts without overwriting previous work.
    with args.output.open('x') as output:
        try:
            with args.database.open('xb'):
                pass
            args.exports.mkdir(exist_ok=False, parents=True)
            audit = json.loads(args.audit_result.read_text())
            serving = json.loads(args.serving_result.read_text())
            inventory = {x['file']: x for x in audit['artifacts']}
            expected_count = audit['totals_per_file_sum']['records']
            if audit['totals_per_file_sum']['rejected_records'] or audit['totals_per_file_sum']['duplicate_occurrences']:
                raise ValueError('This experiment requires audited zero-rejection/duplicate input')
            if serving['files_records'] != {name: x['records'] for name, x in inventory.items()}:
                raise ValueError('Serving/audit per-file counts disagree')
            manifest = {'schema': 'sqlite-storage-experiment-v1', 'mapping': 'identity',
                        'normalization': f'python-unicode-{unicodedata.unidata_version}-NFC-casefold-NFC',
                        'source': 'owner-confirmed-final-CSV',
                        'artifacts': [{'name': name, 'sha256': inventory[name]['sha256'],
                                       'records': inventory[name]['records']}
                                      for name in sorted(inventory)]}
            manifest_bytes = json.dumps(manifest, sort_keys=True, separators=(',', ':')).encode()
            fingerprint = hashlib.sha256(manifest_bytes).digest()
            dataset_uuid = uuid.uuid5(uuid.NAMESPACE_URL, 'slovnyk-storage-probe:' + fingerprint.hex())
            dataset = dataset_uuid.bytes
            conn = sqlite3.connect(args.database, uri=True)
            conn.execute('PRAGMA foreign_keys=OFF')
            conn.execute('PRAGMA cache_size=-8192')
            conn.execute('PRAGMA mmap_size=0')
            conn.execute('PRAGMA temp_store=FILE')
            conn.execute('PRAGMA synchronous=FULL')
            load_schema(conn)
            conn.create_function('tuple_hash', 4, tuple_hash, deterministic=True)
            conn.execute('ATTACH DATABASE ? AS serving',
                         (args.serving_database.resolve().as_uri() + '?mode=ro&immutable=1',))
            if conn.execute('SELECT COUNT(*) FROM serving.translations').fetchone()[0] != expected_count:
                raise ValueError('Serving snapshot record count mismatch')
            now = datetime.now(timezone.utc).isoformat()
            conn.execute('INSERT INTO datasets VALUES(?,?,?,?,?,?,?,?,?)',
                         (dataset, fingerprint, manifest_bytes, 'storage-experiment-v1',
                          'audited-CSV/serving-seed', 'identity', manifest['normalization'], 'loading', now))
            conn.executemany('INSERT INTO dataset_locales VALUES(?,?,NULL)',
                             [(dataset, code) for code in audit['locale_inventory']])
            run_id = uuid.uuid4().bytes
            conn.execute('INSERT INTO import_runs VALUES(?,?,?,?,?,?)', (run_id, dataset, 1, '{}', 'loading', now))
            conn.commit()
            spans = []
            first = 1
            phase = time.monotonic()
            for index, (name, count) in enumerate(serving['files_records'].items(), 1):
                item = inventory[name]
                source_path = args.source / name
                artifact = bytes.fromhex(item['sha256'])
                if file_hash(source_path) != artifact:
                    raise ValueError(f'Source changed: {name}')
                pair = item['pairs'][0]
                sl, tl = pair['from'], pair['to']
                last = first + count - 1
                conn.execute('INSERT INTO dataset_pairs VALUES(?,?,?,?)', (dataset, sl, tl, count))
                conn.execute('INSERT INTO source_files VALUES(?,?,?,?,?,?,?,?,?)',
                             (dataset, artifact, name, 'lossless-csv4-v1', item['compressed_bytes'], 1, 1, first, last))
                conn.execute('''INSERT INTO translations
                    SELECT ?,id,sl,tl,source,target,key,key_hash,tuple_hash(sl,tl,source,target)
                    FROM serving.translations WHERE id BETWEEN ? AND ? ORDER BY id''', (dataset, first, last))
                conn.execute('''INSERT INTO import_records
                    SELECT dataset_id,?,id-?+1,'unique',id,NULL FROM translations
                    WHERE dataset_id=? AND id BETWEEN ? AND ? ORDER BY id''',
                             (artifact, first, dataset, first, last))
                conn.execute('''INSERT INTO translation_origins
                    SELECT dataset_id,?,id-?+1,id,source_locale,target_locale,source_text,target_text,NULL,NULL
                    FROM translations WHERE dataset_id=? AND id BETWEEN ? AND ? ORDER BY id''',
                             (artifact, first, dataset, first, last))
                conn.execute('UPDATE import_runs SET checkpoint=? WHERE id=?',
                             (json.dumps({'completed_file': name, 'last_id': last}), run_id))
                conn.commit()
                spans.append((name, sl, tl, artifact, first, last, count))
                first = last + 1
                if index % 25 == 0:
                    log(f'Loaded provenance {index}/{len(inventory)} files; {first-1} rows; {args.database.stat().st_size} bytes')
            result['phases_s']['bulk_load'] = round(time.monotonic() - phase, 3)
            result['bytes_before_secondary_indexes'] = args.database.stat().st_size
            phase = time.monotonic()
            log('Building UNIQUE tuple hash and exact lookup indexes')
            conn.execute('CREATE UNIQUE INDEX tuple_identity ON translations(dataset_id,tuple_hash)')
            conn.execute('CREATE INDEX exact_lookup ON translations(dataset_id,source_locale,target_locale,key_hash)')
            conn.execute("UPDATE datasets SET state='validating' WHERE id=?", (dataset,))
            conn.execute("UPDATE import_runs SET state='validating' WHERE id=?", (run_id,))
            conn.commit()
            conn.execute('PRAGMA foreign_keys=ON')
            result['phases_s']['secondary_indexes'] = round(time.monotonic() - phase, 3)
            result['bytes_with_secondary_indexes'] = args.database.stat().st_size
            phase = time.monotonic()
            log('Checking all foreign keys')
            violations = conn.execute('PRAGMA foreign_key_check').fetchmany(10)
            if violations:
                raise ValueError(f'Foreign key violations: {violations}')
            result['foreign_key_check'] = 'ok'
            result['phases_s']['foreign_key_check'] = round(time.monotonic() - phase, 3)
            phase = time.monotonic()
            for index, (name, sl, tl, artifact, first, last, count) in enumerate(spans, 1):
                if file_hash(args.source / name) != artifact:
                    raise ValueError(f'Source changed during probe: {name}')
                with gzip.open(args.source / name, 'rt', encoding='utf-8-sig', newline='') as stream:
                    rows = [tuple(row) for row in csv.reader(stream, strict=True)]
                if len(rows) != count:
                    raise ValueError(f'Source record count changed: {name}')
                # Compare every source record to both translation and origin at its ordinal.
                translations = conn.execute('''SELECT id,source_locale,target_locale,source_text,target_text,
                    source_key,key_hash,tuple_hash FROM translations WHERE dataset_id=?
                    AND id BETWEEN ? AND ? ORDER BY id''', (dataset, first, last))
                seen = 0
                for seen, row in enumerate(translations, 1):
                    raw = rows[seen-1]
                    key = normalize(raw[2])
                    if row[:5] != (first+seen-1, *raw) or row[5] != key or row[6] != hashlib.sha256(key.encode()).digest() or row[7] != tuple_hash(*raw):
                        raise ValueError(f'Translation/normalization mismatch: {name}, ordinal {seen}')
                if seen != count:
                    raise ValueError(f'Translation count mismatch: {name}')
                origins = conn.execute('''SELECT ordinal,translation_id,source_locale,target_locale,
                    source_text,target_text,legacy_count,legacy_status FROM translation_origins
                    WHERE dataset_id=? AND artifact_hash=? ORDER BY ordinal''', (dataset, artifact))
                seen = 0
                for seen, row in enumerate(origins, 1):
                    if row != (seen, first+seen-1, *rows[seen-1], None, None):
                        raise ValueError(f'Origin mismatch: {name}, ordinal {seen}')
                if seen != count:
                    raise ValueError(f'Origin count mismatch: {name}')
                expected_digest = hashlib.sha256()
                for raw in sorted(rows, key=lambda row: tuple(value.encode('utf-8') for value in row)):
                    expected_digest.update(tuple_bytes(*raw))
                del rows
                database_digest = hashlib.sha256()
                exported_path = args.exports / name
                with exported_path.open('xb') as binary:
                    with gzip.GzipFile(filename='', mode='wb', fileobj=binary, compresslevel=1, mtime=0) as compressed:
                        with io.TextIOWrapper(compressed, encoding='utf-8', newline='') as text:
                            writer = csv.writer(text, lineterminator='\n')
                            selected = conn.execute('''SELECT source_locale,target_locale,source_text,target_text
                                FROM translations WHERE dataset_id=? AND id BETWEEN ? AND ?
                                ORDER BY source_locale COLLATE BINARY,target_locale COLLATE BINARY,
                                source_text COLLATE BINARY,target_text COLLATE BINARY''', (dataset, first, last))
                            for raw in selected:
                                database_digest.update(tuple_bytes(*raw))
                                writer.writerow(raw)
                    binary.flush()
                    os.fsync(binary.fileno())
                export_digest = hashlib.sha256()
                export_count = 0
                with gzip.open(exported_path, 'rt', encoding='utf-8', newline='') as stream:
                    for raw in csv.reader(stream, strict=True):
                        export_digest.update(tuple_bytes(*raw))
                        export_count += 1
                if expected_digest.digest() != database_digest.digest() or expected_digest.digest() != export_digest.digest() or export_count != count:
                    raise ValueError(f'Canonical per-pair digest mismatch: {name}')
                export_hash = file_hash(exported_path)
                export_id = uuid.uuid5(dataset_uuid, name).bytes
                conn.execute('INSERT INTO exports VALUES(?,?,?,?,?,?,?,?,?)',
                             (dataset, export_id, sl, tl, name, export_hash, exported_path.stat().st_size, count, 1))
                conn.commit()
                result['pairs'].append({'file': name, 'records': count,
                                        'source_db_export_sorted_tuple_sha256': expected_digest.hexdigest(),
                                        'export_sha256': export_hash.hex(),
                                        'export_bytes': exported_path.stat().st_size})
                if index % 25 == 0:
                    log(f'Reconciled source/translations/origins/export {index}/{len(spans)} pairs')
            result['phases_s']['reconcile_and_export'] = round(time.monotonic() - phase, 3)
            result['table_rows'] = {name: conn.execute(f'SELECT COUNT(*) FROM {name}').fetchone()[0]
                                    for name in ['translations', 'import_records', 'translation_origins',
                                                 'source_files', 'dataset_pairs', 'dataset_locales',
                                                 'exports', 'import_issues', 'legacy_query_snapshots']}
            if any(result['table_rows'][name] != expected_count for name in ['translations','import_records','translation_origins']):
                raise ValueError('Record accounting mismatch')
            if conn.execute("SELECT COUNT(*) FROM import_records WHERE outcome!='unique' OR issue_code IS NOT NULL").fetchone()[0]:
                raise ValueError('Unexpected import outcomes')
            result.update(complete=True, dataset_state='validating', active_dataset=None,
                          dataset_id=str(dataset_uuid), experimental_manifest_sha256=fingerprint.hex(),
                          database_bytes=args.database.stat().st_size,
                          exports_bytes=sum(pair['export_bytes'] for pair in result['pairs']),
                          total_s=round(time.monotonic()-started, 3),
                          peak_process_rss_KiB_linux=resource.getrusage(resource.RUSAGE_SELF).ru_maxrss,
                          python=platform.python_version(), sqlite=sqlite3.sqlite_version,
                          unicode=unicodedata.unidata_version)
            conn.close()
        except Exception as exc:
            result.update(error=f'{type(exc).__name__}: {exc}', total_s=round(time.monotonic()-started,3))
            raise
        finally:
            json.dump(result, output, indent=2)
            output.write('\n')
    log(json.dumps({k: v for k, v in result.items() if k != 'pairs'}))


if __name__ == '__main__':
    main()
