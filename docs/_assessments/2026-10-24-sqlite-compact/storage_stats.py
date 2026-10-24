"""Read-only storage accounting and workload query plans."""

import argparse
import json
import time
from pathlib import Path

from probe import connect


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('database', type=Path)
    parser.add_argument('output', type=Path)
    args = parser.parse_args()
    with args.output.open('x') as output:
        started = time.monotonic()
        conn = connect(args.database, readonly=True)
        conn.execute('''CREATE TEMP VIEW probe_translations AS
            SELECT id,source_locale AS sl,target_locale AS tl,source_text AS source,
                   target_text AS target,source_key AS key,key_hash
            FROM main.translations WHERE dataset_id=(SELECT id FROM main.datasets LIMIT 1)''')
        order = ' ORDER BY key COLLATE BINARY,source COLLATE BINARY,target COLLATE BINARY,id LIMIT 51'
        queries = {
            'exact': ('SELECT id,source,target FROM probe_translations WHERE sl=? AND tl=? AND key_hash=? AND key=?'+order,
                      ('en-us','uk-ua',b'x'*32,'test')),
            'prefix': ('SELECT id,source,target FROM probe_translations WHERE sl=? AND tl=? AND key GLOB ?'+order,
                       ('en-us','uk-ua','test*')),
            'infix': ('SELECT id,source,target FROM probe_translations WHERE sl=? AND tl=? AND key GLOB ?'+order,
                      ('en-us','uk-ua','*test*')),
        }
        result = {'query_plans': {name: conn.execute('EXPLAIN QUERY PLAN '+query, params).fetchall()
                                  for name, (query, params) in queries.items()}}
        result['objects'] = {name: {'pages': pages, 'bytes': size}
                             for name, pages, size in conn.execute('SELECT name,pageno,pgsize FROM dbstat WHERE aggregate=TRUE')}
        page_size = conn.execute('PRAGMA page_size').fetchone()[0]
        page_count = conn.execute('PRAGMA page_count').fetchone()[0]
        free_pages = conn.execute('PRAGMA freelist_count').fetchone()[0]
        if conn.execute('PRAGMA auto_vacuum').fetchone()[0] != 0:
            raise ValueError('Pointer-map page accounting is outside this experiment')
        # https://www.sqlite.org/fileformat.html#the_lock_byte_page
        lock_pages = int(page_size*page_count > 1073741824)
        if sum(row['pages'] for row in result['objects'].values()) + free_pages + lock_pages != page_count:
            raise ValueError('Page accounting mismatch')
        result.update(page_size=page_size, page_count=page_count, free_pages=free_pages, lock_byte_pages=lock_pages,
                      bytes=page_size*page_count, elapsed_s=round(time.monotonic()-started,3))
        conn.close()
        json.dump(result, output, indent=2)
        output.write('\n')
    print(json.dumps(result))


if __name__ == '__main__':
    main()
