"""Synthetic integrity and round-trip checks for the storage experiment."""

import contextlib
import csv
import gzip
import hashlib
import io
import json
import sqlite3
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import probe


class StorageTests(unittest.TestCase):
    def fixture(self, root):
        rows = [('en-us', 'uk-ua', 'Straße', 'вулиця'),
                ('en-us', 'uk-ua', 'STRASSE', 'дорога'),
                ('en-us', 'uk-ua', ' a*? ', 'x\r\ny"z')]
        sources = root / 'source'
        sources.mkdir()
        name = 'slovnyk_en-us_uk-ua.csv.gz'
        with gzip.open(sources / name, 'wt', encoding='utf-8', newline='') as output:
            csv.writer(output, lineterminator='\n').writerows(rows)
        compressed = (sources / name).read_bytes()
        artifact = {'file': name, 'records': 3, 'compressed_bytes': len(compressed),
                    'sha256': hashlib.sha256(compressed).hexdigest(),
                    'pairs': [{'from': 'en-us', 'to': 'uk-ua', 'records': 3}]}
        audit = {'artifacts': [artifact], 'totals_per_file_sum':
                 {'records': 3, 'rejected_records': 0, 'duplicate_occurrences': 0},
                 'locale_inventory': ['en-us', 'uk-ua']}
        (root / 'audit.json').write_text(json.dumps(audit))
        (root / 'serving.json').write_text(json.dumps({'files_records': {name: 3}}))
        conn = sqlite3.connect(root / 'serving.sqlite3')
        conn.execute('CREATE TABLE translations(id INTEGER PRIMARY KEY,sl,tl,source,target,key,key_hash)')
        for index, row in enumerate(rows, 1):
            key = probe.normalize(row[2])
            conn.execute('INSERT INTO translations VALUES(?,?,?,?,?,?,?)',
                         (index, *row, key, hashlib.sha256(key.encode()).digest()))
        conn.commit()
        conn.close()
        return rows, [str(x) for x in [sources, root/'serving.sqlite3', root/'audit.json',
                     root/'serving.json', root/'model.sqlite3', root/'exports', root/'result.json']]

    def run_fixture(self, args):
        with patch.object(sys, 'argv', ['probe.py', *args]), contextlib.redirect_stdout(io.StringIO()):
            probe.main()

    def test_full_round_trip_preserves_raw_values_and_does_not_publish(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            rows, args = self.fixture(root)
            self.run_fixture(args)
            report = json.loads((root/'result.json').read_text())
            self.assertTrue(report['complete'])
            self.assertEqual(report['table_rows']['translation_origins'], 3)
            self.assertEqual(report['foreign_key_check'], 'ok')
            conn = sqlite3.connect(root/'model.sqlite3')
            self.assertEqual(conn.execute('SELECT state FROM datasets').fetchone()[0], 'validating')
            self.assertEqual(conn.execute('SELECT dataset_id,generation FROM active_dataset').fetchone(), (None, 0))
            self.assertEqual(conn.execute('SELECT legacy_count,legacy_status FROM translation_origins').fetchall(), [(None,None)]*3)
            self.assertEqual(conn.execute('SELECT COUNT(DISTINCT source_key) FROM translations').fetchone()[0], 2)
            conn.close()
            with gzip.open(root/'exports'/'slovnyk_en-us_uk-ua.csv.gz', 'rt', encoding='utf-8', newline='') as stream:
                exported = [tuple(row) for row in csv.reader(stream)]
            self.assertEqual(sorted(exported), sorted(rows))

    def test_hash_collision_stops_before_validation_and_publication(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            _, args = self.fixture(root)
            with patch.object(probe, 'tuple_hash', return_value=b'X'*32):
                with self.assertRaises(sqlite3.IntegrityError):
                    self.run_fixture(args)
            result = json.loads((root/'result.json').read_text())
            self.assertFalse(result['complete'])
            conn = sqlite3.connect(root/'model.sqlite3')
            self.assertIsNone(conn.execute('SELECT dataset_id FROM active_dataset').fetchone()[0])
            conn.close()

    def test_foreign_keys_reject_cross_dataset_provenance(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            _, args = self.fixture(root)
            self.run_fixture(args)
            conn = sqlite3.connect(root/'model.sqlite3')
            conn.execute('PRAGMA foreign_keys=ON')
            dataset_row = list(conn.execute('SELECT * FROM datasets').fetchone())
            dataset_row[0] = b'2'*16
            dataset_row[1] = b'3'*32
            conn.execute('INSERT INTO datasets VALUES(?,?,?,?,?,?,?,?,?)', dataset_row)
            artifact = conn.execute('SELECT artifact_hash FROM source_files').fetchone()[0]
            with self.assertRaises(sqlite3.IntegrityError):
                conn.execute("INSERT INTO import_records VALUES(?,?,1,'unique',1,NULL)", (dataset_row[0],artifact))
            conn.rollback()
            conn.close()

    def test_tuple_digest_encoding_has_length_boundaries(self):
        expected = b''.join(b'\0'*7+b'\1'+value for value in [b'a',b'b',b'c',b'd'])
        self.assertEqual(probe.tuple_bytes('a','b','c','d'), expected)
        self.assertEqual(probe.tuple_hash('a','b','c','d'), hashlib.sha256(expected).digest())
        self.assertNotEqual(probe.tuple_bytes('a','b','ab','c'), probe.tuple_bytes('a','b','a','bc'))


if __name__ == '__main__':
    unittest.main()
