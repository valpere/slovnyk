"""Read-only checks of completed experiment evidence and ledger mappings."""

import argparse
import hashlib
import json
import sqlite3
import time
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('database', type=Path)
    parser.add_argument('result', type=Path)
    parser.add_argument('audit', type=Path)
    parser.add_argument('output', type=Path)
    args = parser.parse_args()
    started = time.monotonic()
    result = json.loads(args.result.read_text())
    audit = json.loads(args.audit.read_text())
    assert result['complete'] and result['foreign_key_check'] == 'ok'
    expected = {row['file']: row['records'] for row in audit['artifacts']}
    assert len(result['pairs']) == len(expected)
    assert {row['file']: row['records'] for row in result['pairs']} == expected
    conn = sqlite3.connect(args.database.resolve().as_uri() + '?mode=ro&immutable=1', uri=True)
    conn.execute('PRAGMA cache_size=-8192')
    conn.execute('PRAGMA mmap_size=0')
    datasets = conn.execute('SELECT id,state FROM datasets').fetchall()
    assert len(datasets) == 1 and datasets[0][1] == 'validating'
    dataset = datasets[0][0]
    assert conn.execute('SELECT dataset_id,generation FROM active_dataset').fetchall() == [(None, 0)]
    assert conn.execute('SELECT COUNT(*) FROM publication_events').fetchone()[0] == 0
    files = conn.execute('''SELECT relative_name,artifact_hash,first_translation_id,last_translation_id
        FROM source_files WHERE dataset_id=?''', (dataset,)).fetchall()
    assert len(files) == len(expected)
    checked = 0
    for name, artifact, first, last in files:
        count, invalid = conn.execute('''SELECT COUNT(*),COALESCE(SUM(
            outcome!='unique' OR issue_code IS NOT NULL OR translation_id IS NULL
            OR translation_id!=?+ordinal-1 OR ordinal<1 OR ordinal>?),0)
            FROM import_records WHERE dataset_id=? AND artifact_hash=?''',
            (first, last-first+1, dataset, artifact)).fetchone()
        assert count == expected[name] == last-first+1 and invalid == 0, name
        checked += count
    stats = {
        'validated': True,
        'ledger_records_checked': checked,
        'files_checked': len(files),
        'dataset_state': datasets[0][1],
        'active_dataset': None,
        'generation': 0,
        'publication_events': 0,
        'elapsed_s': round(time.monotonic()-started, 3),
        'tool_sha256': {path.name: hashlib.sha256(path.read_bytes()).hexdigest()
                        for path in sorted(Path(__file__).parent.glob('*.py'))},
    }
    conn.close()
    with args.output.open('x') as stream:
        json.dump(stats, stream, indent=2)
        stream.write('\n')
    print(json.dumps(stats))


if __name__ == '__main__':
    main()
