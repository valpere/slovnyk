"""Record query plans before/after SQLite statistics on the closed local prototype."""

import argparse
import json
import sqlite3
import time
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('database', type=Path)
    parser.add_argument('output', type=Path)
    args = parser.parse_args()
    with args.output.open('x') as output:
        conn = sqlite3.connect(args.database.resolve().as_uri()+'?mode=rw', uri=True)
        conn.execute('PRAGMA cache_size=-8192')
        conn.execute('PRAGMA mmap_size=0')
        dataset = conn.execute('SELECT id FROM datasets').fetchone()[0]
        order = ' ORDER BY source_key COLLATE BINARY,source_text COLLATE BINARY,target_text COLLATE BINARY,id LIMIT 51'
        queries = {
            'exact': ('SELECT id,source_text,target_text FROM translations WHERE dataset_id=? AND source_locale=? AND target_locale=? AND key_hash=? AND source_key=?'+order,
                      (dataset,'en-us','uk-ua',b'x'*32,'test')),
            'prefix': ('SELECT id,source_text,target_text FROM translations WHERE dataset_id=? AND source_locale=? AND target_locale=? AND source_key GLOB ?'+order,
                       (dataset,'en-us','uk-ua','test*')),
            'infix': ('SELECT id,source_text,target_text FROM translations WHERE dataset_id=? AND source_locale=? AND target_locale=? AND source_key GLOB ?'+order,
                      (dataset,'en-us','uk-ua','*test*')),
        }
        def plans():
            return {name: conn.execute('EXPLAIN QUERY PLAN '+query, params).fetchall()
                    for name, (query, params) in queries.items()}
        result = {'before': plans(), 'bytes_before': args.database.stat().st_size}
        started = time.monotonic()
        conn.execute('ANALYZE')
        conn.commit()
        result.update(after=plans(), elapsed_s=round(time.monotonic()-started,3),
                      bytes_after=args.database.stat().st_size,
                      limitation='Only optimizer statistics changed; full-corpus source reconciliation preceded this step')
        conn.close()
        json.dump(result, output, indent=2)
        output.write('\n')
    print(json.dumps(result))


if __name__ == '__main__':
    main()
