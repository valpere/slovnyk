"""Local recovery rehearsal for a closed, immutable experimental SQLite file."""

import argparse
import hashlib
import json
import shutil
import sqlite3
import time
from pathlib import Path


def sha256(path):
    digest = hashlib.sha256()
    with path.open('rb') as stream:
        for chunk in iter(lambda: stream.read(8 * 1024 * 1024), b''):
            digest.update(chunk)
    return digest.hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source', type=Path)
    parser.add_argument('restored', type=Path)
    parser.add_argument('output', type=Path)
    args = parser.parse_args()
    started = time.monotonic()
    with args.output.open('x') as output:
        with args.restored.open('xb'):
            pass
        source_hash = sha256(args.source)
        before = time.monotonic()
        shutil.copyfile(args.source, args.restored)
        copy_seconds = time.monotonic() - before
        restored_hash = sha256(args.restored)
        print('Copy and SHA-256 verification complete; running SQLite quick_check', flush=True)
        conn = sqlite3.connect(args.restored.resolve().as_uri() + '?mode=ro&immutable=1', uri=True)
        checks = [row[0] for row in conn.execute('PRAGMA quick_check')]
        records = conn.execute('SELECT COUNT(*) FROM translations').fetchone()[0]
        conn.close()
        result = {'source_sha256': source_hash, 'restored_sha256': restored_hash,
                  'hashes_equal': source_hash == restored_hash,
                  'source_bytes': args.source.stat().st_size,
                  'restored_bytes': args.restored.stat().st_size,
                  'copy_s': round(copy_seconds, 3),
                  'total_s': round(time.monotonic() - started, 3),
                  'sqlite_quick_check': checks, 'restored_records': records,
                  'limitations': ['Same developer host and filesystem, not an off-host backup',
                                  'Closed experimental DB, not a live production backup',
                                  'No service, DNS, secrets or target-server recovery tested',
                                  'quick_check is not the full SQLite integrity_check']}
        json.dump(result, output, indent=2)
        output.write('\n')
    if not result['hashes_equal'] or checks != ['ok']:
        raise SystemExit('Local recovery checks failed')
    print(json.dumps(result), flush=True)


if __name__ == '__main__':
    main()
