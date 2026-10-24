"""Local SQLite feasibility probe; not a production importer or load test."""

import argparse
import csv
import gzip
import hashlib
import json
import math
import platform
import resource
import sqlite3
import time
import unicodedata
from pathlib import Path


def normalize(value):
    return unicodedata.normalize("NFC", unicodedata.normalize("NFC", value).casefold())


def digest(value):
    return hashlib.sha256(value.encode("utf-8")).digest()


def percentile(values, fraction):
    return sorted(values)[max(0, math.ceil(len(values) * fraction) - 1)]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("source", type=Path)
    parser.add_argument("database", type=Path)
    parser.add_argument("--files", type=int, default=4)
    parser.add_argument("--repeats", type=int, default=50)
    args = parser.parse_args()
    # Refuse to overwrite any existing artifact.
    with args.database.open("xb"):
        pass
    csv.field_size_limit(16 * 1024 * 1024)
    files = sorted(args.source.glob("*.csv.gz"), key=lambda p: (-p.stat().st_size, p.name))[: args.files]
    conn = sqlite3.connect(args.database)
    conn.execute("PRAGMA cache_size=-8192")
    conn.execute("PRAGMA mmap_size=0")
    conn.execute("PRAGMA temp_store=FILE")
    conn.execute("""CREATE TABLE translations (
        id INTEGER PRIMARY KEY, sl TEXT NOT NULL, tl TEXT NOT NULL,
        source TEXT NOT NULL, target TEXT NOT NULL, key TEXT NOT NULL,
        key_hash BLOB NOT NULL
    )""")
    started = time.monotonic()
    counts = {}
    samples = []
    for path in files:
        count = 0
        batch = []
        with gzip.open(path, "rt", encoding="utf-8-sig", newline="") as stream:
            for row in csv.reader(stream, strict=True):
                if len(row) != 4:
                    raise ValueError(f"Unexpected field count in {path.name}")
                sl, tl, source, target = row
                key = normalize(source)
                batch.append((sl, tl, source, target, key, digest(key)))
                if count % 5000 == 0 and len(key) >= 4 and not any(c in key for c in "*?["):
                    samples.append((sl, tl, key))
                count += 1
                if len(batch) >= 5000:
                    conn.executemany("INSERT INTO translations(sl,tl,source,target,key,key_hash) VALUES (?,?,?,?,?,?)", batch)
                    conn.commit()
                    batch.clear()
            if batch:
                conn.executemany("INSERT INTO translations(sl,tl,source,target,key,key_hash) VALUES (?,?,?,?,?,?)", batch)
                conn.commit()
        counts[path.name] = count
    import_s = time.monotonic() - started
    conn.execute("CREATE INDEX exact_hash ON translations(sl,tl,key_hash)")
    conn.execute("CREATE INDEX pair_only ON translations(sl,tl)")
    conn.commit()
    base_bytes = args.database.stat().st_size
    queries = []
    selected_samples = samples[:: max(1, len(samples) // args.repeats)][: args.repeats]
    for sl, tl, key in selected_samples:
        # SQLite GLOB is case-sensitive; escape its additional '[' syntax.
        escaped = key.replace("[", "[[]")
        queries.append(("exact", (sl, tl, digest(key), key)))
        queries.append(("prefix", (sl, tl, key[:3].replace("[", "[[]") + "*")))
        queries.append(("infix", (sl, tl, "*" + escaped + "*")))
    order = " ORDER BY key COLLATE BINARY,source COLLATE BINARY,target COLLATE BINARY,id LIMIT 51"
    exact_sql = "SELECT id,source,target FROM translations WHERE sl=? AND tl=? AND key_hash=? AND key=?" + order
    mask_sql = "SELECT id,source,target FROM translations WHERE sl=? AND tl=? AND key GLOB ?" + order

    def measure():
        timings = {kind: [] for kind in ("exact", "prefix", "infix")}
        plans = {}
        for kind, params in queries:
            sql = exact_sql if kind == "exact" else mask_sql
            if kind not in plans:
                plans[kind] = [row[3] for row in conn.execute("EXPLAIN QUERY PLAN " + sql, params)]
            before = time.perf_counter()
            conn.execute(sql, params).fetchall()
            timings[kind].append((time.perf_counter() - before) * 1000)
        return {
            kind: {
                "queries": len(values),
                "p50_ms": round(percentile(values, 0.5), 3),
                "p95_ms": round(percentile(values, 0.95), 3),
                "max_ms": round(max(values), 3),
                "plan": plans[kind],
            }
            for kind, values in timings.items()
        }

    bounded_indexes = measure()
    before = time.monotonic()
    conn.execute("CREATE INDEX lookup_order ON translations(sl,tl,key COLLATE BINARY,source COLLATE BINARY,target COLLATE BINARY,id)")
    conn.commit()
    index_s = time.monotonic() - before
    conn.execute("ANALYZE")
    conn.commit()
    covering_index = measure()
    conn.close()
    readonly = sqlite3.connect(args.database.resolve().as_uri() + "?mode=ro&immutable=1", uri=True)
    readonly_count = readonly.execute("SELECT COUNT(*) FROM translations").fetchone()[0]
    readonly.close()
    print(json.dumps({
        "python": platform.python_version(),
        "sqlite": sqlite3.sqlite_version,
        "unicode": unicodedata.unidata_version,
        "platform": platform.platform(),
        "files_records": counts,
        "records": sum(counts.values()),
        "sample_pairs": sorted({f"{sl}->{tl}" for sl, tl, _ in selected_samples}),
        "import_s": round(import_s, 3),
        "bytes_with_hash_and_pair_indexes": base_bytes,
        "bytes_with_additional_lookup_index": args.database.stat().st_size,
        "lookup_index_build_s": round(index_s, 3),
        "max_process_rss_kib_linux": resource.getrusage(resource.RUSAGE_SELF).ru_maxrss,
        "bounded_indexes": bounded_indexes,
        "additional_covering_index": covering_index,
        "readonly_open_records": readonly_count,
        "limitations": [
            "Local developer machine, not Oracle Micro; OS page cache is uncontrolled",
            "8 MiB SQLite cache per connection is not a process/OS memory limit",
            "Single connection; serial SQL calls, no HTTP or 10 requests/s load test",
            "Four largest compressed files, not the complete corpus",
            "No production deduplication, provenance, dataset publication, or parser acceptance",
            "Covering index is an experiment, not an approved change to DATA_MODEL",
            "Python Unicode normalization is not a selected production implementation",
        ],
    }, indent=2))


if __name__ == "__main__":
    main()
