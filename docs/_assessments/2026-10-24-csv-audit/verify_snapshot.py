"""Independent SHA-256 and strict gzip-member verification of the audited snapshot.

Python gzip accepts zero padding; IMPORT_CONTRACT disallows trailing non-gzip
bytes, so verify the exact member boundaries separately using zlib.
"""

import argparse
import hashlib
import json
import time
import zlib
from pathlib import Path


def strict_gzip_size(data):
    size = members = 0
    while data:
        if not data.startswith(b'\x1f\x8b'):
            raise ValueError('non-gzip bytes outside a member')
        decoder = zlib.decompressobj(31)
        unpacked = decoder.decompress(data)
        if not decoder.eof:
            raise ValueError('incomplete gzip member')
        size += len(unpacked)
        members += 1
        data = decoder.unused_data
    if not members:
        raise ValueError('no gzip members')
    return size, members


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source', type=Path)
    parser.add_argument('audit_result', type=Path)
    parser.add_argument('output', type=Path)
    args = parser.parse_args()
    audit = json.loads(args.audit_result.read_text())
    started = time.monotonic()
    with args.output.open('x') as output:
        members = 0
        for item in audit['artifacts']:
            compressed = (args.source / item['file']).read_bytes()
            if hashlib.sha256(compressed).hexdigest() != item['sha256']:
                raise ValueError(f"Source changed: {item['file']}")
            size, count = strict_gzip_size(compressed)
            if size != item['decompressed_bytes']:
                raise ValueError(f"Decompressed size mismatch: {item['file']}")
            members += count
        observed = {p.name for p in args.source.glob('*.csv.gz')}
        expected = {item['file'] for item in audit['artifacts']}
        if observed != expected:
            raise ValueError('Source file inventory changed')
        result = {'verified_files': len(expected), 'gzip_members': members,
                  'all_sha256_unchanged': True, 'strict_member_boundaries': True,
                  'elapsed_s': round(time.monotonic() - started, 3)}
        json.dump(result, output, indent=2)
        output.write('\n')
    print(json.dumps(result))


if __name__ == '__main__':
    main()
