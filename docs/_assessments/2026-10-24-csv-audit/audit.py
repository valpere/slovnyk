"""Read-only corpus audit; diagnostic tool, not the production importer."""

import argparse
import csv
import gzip
import hashlib
import io
import json
import platform
import re
import resource
import time
import unicodedata
import zlib
from collections import Counter
from pathlib import Path


FIELD = r'(?:"(?:[^"]|"")*"|[^",\r\n]*)'
RECORD = re.compile(FIELD + r'(?:,' + FIELD + r')*(?:\r\n|\n|\Z)')
NAME = re.compile(r'slovnyk_([^_]+)_([^_]+)\.csv\.gz\Z')


def normalize(value):
    return unicodedata.normalize('NFC', unicodedata.normalize('NFC', value).casefold())


def audit_file(path, known_locales):
    compressed = path.read_bytes()
    result = {'file': path.name, 'sha256': hashlib.sha256(compressed).hexdigest(),
              'compressed_bytes': len(compressed), 'complete': False}
    issues = Counter()
    samples = {}

    def issue(reason, ordinal, **details):
        issues[reason] += 1
        if len(samples.setdefault(reason, [])) < 8:
            samples[reason].append({'ordinal': ordinal, **details})

    try:
        # gzip.decompress validates every member including trailer and trailing data.
        data = gzip.decompress(compressed)
        result['decompressed_bytes'] = len(data)
        result['bom'] = data.startswith(b'\xef\xbb\xbf')
        text = data.decode('utf-8-sig', errors='strict')
        position = ordinal = max_record = 0
        oversized_records = set()
        while position < len(text):
            match = RECORD.match(text, position)
            if match is None or match.end() == position:
                raise ValueError(f'lossless CSV grammar failure before ordinal {ordinal + 1}')
            ordinal += 1
            record_bytes = len(match.group().encode('utf-8'))
            max_record = max(max_record, record_bytes)
            if record_bytes > 1024 * 1024:
                issue('record_over_1MiB', ordinal)
                oversized_records.add(ordinal)
            position = match.end()
        result['lexical_records'] = ordinal
        result['max_serialized_record_bytes_including_terminator'] = max_record
        if len(data) > 16 * 1024**3:
            issue('artifact_over_16GiB', 0)
        filename = NAME.fullmatch(path.name)
        expected = filename.groups() if filename else None
        if expected is None:
            issue('unrecognized_filename', 0)
        seen = set()
        first_spelling = {}
        variants = set()
        pairs = Counter()
        max_bytes = [0] * 4
        max_codepoints = [0] * 4
        accepted = rejected = duplicates = records = 0
        reader = csv.reader(io.StringIO(text, newline=''), strict=True)
        for ordinal, row in enumerate(reader, 1):
            records += 1
            if len(row) != 4:
                issue('wrong_field_count', ordinal, fields=len(row))
                rejected += 1
                continue
            invalid = ordinal in oversized_records
            for i, value in enumerate(row):
                size = len(value.encode('utf-8'))
                max_bytes[i] = max(max_bytes[i], size)
                max_codepoints[i] = max(max_codepoints[i], len(value))
                for reason, condition in [('empty_field', not value),
                                          ('nul_in_field', '\0' in value),
                                          ('field_over_64KiB', size > 65536)]:
                    if condition:
                        issue(reason, ordinal, field=i, bytes=size)
                        invalid = True
            sl, tl, source, target = row
            pairs[(sl, tl)] += 1
            if expected is not None and (sl, tl) != expected:
                issue('pair_differs_from_filename', ordinal)
            if sl not in known_locales or tl not in known_locales:
                issue('locale_not_in_filename_inventory', ordinal)
                invalid = True
            if source != source.strip():
                issue('source_edge_whitespace', ordinal)
            if len(source) > 1024 or len(source.encode('utf-8')) > 8192:
                issue('source_exceeds_query_limit', ordinal)
            if any(ord(c) < 32 and c not in '\t\n\r' or ord(c) == 127 for c in source):
                issue('source_has_forbidden_query_control', ordinal)
            if '*' in source or '?' in source:
                issue('source_contains_mask_metacharacter', ordinal)
            if '\r' in source or '\r' in target:
                issue('text_contains_CR', ordinal)
            if '\n' in source or '\n' in target:
                issue('text_contains_LF', ordinal)
            # Exact tuple equality, without hash-only duplicate decisions.
            value = tuple(row)
            if invalid:
                rejected += 1
            elif value in seen:
                duplicates += 1
            else:
                accepted += 1
            seen.add(value)
            key = (sl, tl, normalize(source))
            previous = first_spelling.setdefault(key, source)
            if previous != source:
                variants.add((key, source))
        if records != result['lexical_records']:
            raise ValueError('lexical and csv.reader record counts disagree')
        result.update(complete=True, records=records, accepted_unique=accepted,
                      duplicate_occurrences=duplicates, rejected_records=rejected,
                      distinct_raw_tuples=len(seen),
                      normalized_source_keys=len(first_spelling),
                      extra_source_spellings_for_same_normalized_key=len(variants),
                      pairs=[{'from': a, 'to': b, 'records': n}
                             for (a, b), n in sorted(pairs.items())],
                      max_field_utf8_bytes=max_bytes, max_field_codepoints=max_codepoints)
    except (ValueError, UnicodeError, OSError, EOFError, csv.Error, zlib.error) as exc:
        result['fatal_error'] = f'{type(exc).__name__}: {exc}'
    result.update(issues=dict(issues), samples=samples)
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source', type=Path)
    parser.add_argument('output', type=Path)
    args = parser.parse_args()
    csv.field_size_limit(32 * 1024 * 1024)
    files = sorted(args.source.glob('*.csv.gz'))
    if not files:
        parser.error('no CSV.gz sources found')
    known = {code for p in files if (m := NAME.fullmatch(p.name)) for code in m.groups()}
    started = time.monotonic()
    results = []
    # Reserve output before doing work; never overwrite prior evidence.
    with args.output.open('x', encoding='utf-8') as output:
        for index, path in enumerate(files, 1):
            item = audit_file(path, known)
            results.append(item)
            if index % 50 == 0 or not item['complete']:
                print(f'{index}/{len(files)} files, {time.monotonic()-started:.1f}s', flush=True)
        totals = Counter()
        for item in results:
            for key in ['records', 'accepted_unique', 'duplicate_occurrences',
                        'rejected_records', 'compressed_bytes', 'decompressed_bytes',
                        'extra_source_spellings_for_same_normalized_key']:
                totals[key] += item.get(key, 0)
        issues = Counter()
        for item in results:
            issues.update(item['issues'])
        final = {'python': platform.python_version(), 'unicode': unicodedata.unidata_version,
                 'platform': platform.platform(), 'elapsed_s': round(time.monotonic()-started, 3),
                 'peak_rss_KiB_linux': resource.getrusage(resource.RUSAGE_SELF).ru_maxrss,
                 'files': len(files), 'completed_files': sum(x['complete'] for x in results),
                 'locale_inventory': sorted(known), 'totals_per_file_sum': dict(totals),
                 'issues': dict(issues), 'artifacts': results}
        json.dump(final, output, ensure_ascii=False, indent=2)
        output.write('\n')
    print(json.dumps({k: v for k, v in final.items() if k != 'artifacts'}, ensure_ascii=False), flush=True)


if __name__ == '__main__':
    main()
