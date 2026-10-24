"""Add an ordered covering index to a verified copy of the local prototype."""

import argparse
import hashlib
import json
import os
import sqlite3
import time
from pathlib import Path


def file_hash(path):
    digest = hashlib.sha256()
    with path.open('rb') as stream:
        for chunk in iter(lambda: stream.read(8 * 1024 * 1024), b''):
            digest.update(chunk)
    return digest.hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source', type=Path)
    parser.add_argument('database', type=Path)
    parser.add_argument('output', type=Path)
    args = parser.parse_args()
    result = {'complete': False, 'phases_s': {}}
    args.database.parent.mkdir(parents=True, exist_ok=True)
    with args.output.open('x') as output:
        try:
            started = time.monotonic()
            digest = hashlib.sha256()
            with args.source.open('rb') as source, args.database.open('xb') as destination:
                for chunk in iter(lambda: source.read(8 * 1024 * 1024), b''):
                    digest.update(chunk)
                    destination.write(chunk)
                destination.flush()
                os.fsync(destination.fileno())
            result['source_sha256'] = digest.hexdigest()
            result['copy_sha256_before_index'] = file_hash(args.database)
            if result['source_sha256'] != result['copy_sha256_before_index']:
                raise ValueError('Copy hash mismatch')
            result['phases_s']['copy_and_verify'] = round(time.monotonic()-started, 3)
            result['bytes_before'] = args.database.stat().st_size
            print('Verified independent copy; building covering index', flush=True)
            conn = sqlite3.connect(args.database)
            conn.execute('PRAGMA cache_size=-8192')
            conn.execute('PRAGMA mmap_size=0')
            conn.execute('PRAGMA temp_store=FILE')
            started = time.monotonic()
            statement = '''CREATE INDEX lookup_order ON translations(
                dataset_id,source_locale,target_locale,source_key COLLATE BINARY,
                source_text COLLATE BINARY,target_text COLLATE BINARY,id)'''
            conn.execute(statement)
            conn.commit()
            result['index_sql'] = statement
            result['phases_s']['index_build'] = round(time.monotonic()-started, 3)
            started = time.monotonic()
            conn.execute('ANALYZE lookup_order')
            conn.commit()
            result['phases_s']['analyze_new_index'] = round(time.monotonic()-started, 3)
            conn.execute('''CREATE TEMP VIEW probe_translations AS
                SELECT id,source_locale AS sl,target_locale AS tl,source_text AS source,
                       target_text AS target,source_key AS key,key_hash
                FROM main.translations
                WHERE dataset_id=(SELECT id FROM main.datasets LIMIT 1)''')
            order = ' ORDER BY key COLLATE BINARY,source COLLATE BINARY,target COLLATE BINARY,id LIMIT 51'
            queries = {
                'exact': ('SELECT id,source,target FROM probe_translations WHERE sl=? AND tl=? AND key_hash=? AND key=?'+order,
                          ('en-us','uk-ua',b'x'*32,'test')),
                'prefix': ('SELECT id,source,target FROM probe_translations WHERE sl=? AND tl=? AND key GLOB ?'+order,
                           ('en-us','uk-ua','test*')),
                'infix': ('SELECT id,source,target FROM probe_translations WHERE sl=? AND tl=? AND key GLOB ?'+order,
                          ('en-us','uk-ua','*test*')),
            }
            result['query_plans'] = {
                name: conn.execute('EXPLAIN QUERY PLAN '+query, params).fetchall()
                for name, (query, params) in queries.items()}
            result['translations'] = conn.execute('SELECT COUNT(*) FROM translations').fetchone()[0]
            result['dataset_states'] = [row[0] for row in conn.execute('SELECT state FROM datasets')]
            result['active_pointer'] = conn.execute('SELECT dataset_id,generation FROM active_dataset').fetchone()
            if result['translations'] != 56455068 or result['dataset_states'] != ['validating'] or result['active_pointer'] != (None, 0):
                raise ValueError('Unexpected source dataset or publication state')
            conn.close()
            result.update(complete=True, bytes_after=args.database.stat().st_size,
                          bytes_added=args.database.stat().st_size-result['bytes_before'])
        except Exception as exc:
            result['error'] = f'{type(exc).__name__}: {exc}'
            raise
        finally:
            json.dump(result, output, indent=2)
            output.write('\n')
    print(json.dumps(result))


if __name__ == '__main__':
    main()
