"""Exploratory SQL-only workload on the local experimental database, not HTTP."""

import argparse
import csv
import gzip
import hashlib
import json
import math
import sqlite3
import threading
import time
import unicodedata
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path


def percentile(values, fraction):
    return sorted(values)[max(0, math.ceil(len(values) * fraction) - 1)]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source', type=Path)
    parser.add_argument('database', type=Path)
    parser.add_argument('output', type=Path)
    args = parser.parse_args()
    samples = []
    for path in sorted(args.source.glob('*.csv.gz'), key=lambda p: (-p.stat().st_size, p.name))[:4]:
        count = 0
        with gzip.open(path, 'rt', encoding='utf-8-sig', newline='') as stream:
            for sl, tl, source, _ in csv.reader(stream, strict=True):
                key = unicodedata.normalize('NFC', unicodedata.normalize('NFC', source).casefold())
                if len(key) >= 4 and not any(c in key for c in '*?['):
                    samples.append((sl, tl, key))
                    count += 1
                    if count == 16:
                        break
    if len(samples) != 64:
        raise ValueError('Cannot construct expected workload samples')
    local = threading.local()
    order = ' ORDER BY key COLLATE BINARY,source COLLATE BINARY,target COLLATE BINARY,id LIMIT 51'
    exact = 'SELECT id,source,target FROM translations WHERE sl=? AND tl=? AND key_hash=? AND key=?' + order
    mask = 'SELECT id,source,target FROM translations WHERE sl=? AND tl=? AND key GLOB ?' + order

    def query(index, scheduled):
        if not hasattr(local, 'conn'):
            local.conn = sqlite3.connect(args.database.resolve().as_uri() + '?mode=ro&immutable=1', uri=True)
            local.conn.execute('PRAGMA cache_size=-8192')
            local.conn.execute('PRAGMA mmap_size=0')
            local.conn.execute('PRAGMA temp_store=FILE')
        sl, tl, key = samples[index % len(samples)]
        kind = 'exact' if index % 10 < 8 else ('prefix' if index % 10 == 8 else 'infix')
        started = time.monotonic()
        local.conn.set_progress_handler(lambda: int(time.monotonic() - started >= 2), 1000)
        error = None
        if kind == 'exact':
            if index % 20 < 4:
                key += '_slovnyk_probe_missing_20261024'
            params = (sl, tl, hashlib.sha256(key.encode()).digest(), key)
        else:
            params = (sl, tl, key[:3] + '*' if kind == 'prefix' else '*' + key + '*')
        try:
            rows = len(local.conn.execute(exact if kind == 'exact' else mask, params).fetchall())
        except sqlite3.Error as exc:
            error = type(exc).__name__ + ': ' + str(exc)
            rows = None
        ended = time.monotonic()
        return {'kind': kind, 'latency_ms': (ended - scheduled) * 1000,
                'sql_ms': (ended - started) * 1000, 'error': error, 'rows': rows}

    # Reserve evidence path and refuse to overwrite prior results.
    with args.output.open('x') as output:
        results = []
        started = time.monotonic()
        with ThreadPoolExecutor(max_workers=4) as pool:
            for index in range(300):
                scheduled = started + index / 10
                time.sleep(max(0, scheduled - time.monotonic()))
                results.append(pool.submit(query, index, scheduled))
            rows = [future.result() for future in results]
        grouped = {}
        for kind in ['exact', 'prefix', 'infix', 'all']:
            selected = rows if kind == 'all' else [r for r in rows if r['kind'] == kind]
            values = [r['latency_ms'] for r in selected]
            grouped[kind] = {'queries': len(selected),
                             'p50_ms': round(percentile(values, .5), 3),
                             'p95_ms': round(percentile(values, .95), 3),
                             'max_ms': round(max(values), 3),
                             'errors': sum(r['error'] is not None for r in selected)}
        result = {'offered_queries_per_second': 10, 'queries': 300, 'max_workers': 4,
                  'elapsed_s': round(time.monotonic() - started, 3),
                  'mix': '80% exact (including misses), 10% prefix, 10% infix',
                  'sample_pairs': 4, 'metrics_including_scheduling_queue': grouped,
                  'errors': [r['error'] for r in rows if r['error']],
                  'limitations': ['Developer machine; uncontrolled warm OS page cache',
                                  'Samples only from four largest pairs in full-corpus database',
                                  'SQL only, no HTTP/rendering/cursors/publication or bot traffic',
                                  'Exploratory profile, not an approved acceptance workload',
                                  'SQLite progress callback is not a hard wall-clock I/O timeout']}
        json.dump(result, output, indent=2)
        output.write('\n')
    print(json.dumps(result))


if __name__ == '__main__':
    main()
