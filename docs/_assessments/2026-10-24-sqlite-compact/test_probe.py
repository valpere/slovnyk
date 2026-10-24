"""Synthetic checks of reversible physical keys and cross-dataset constraints."""

import contextlib
import importlib.util
import io
import json
import sqlite3
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import probe


def module(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    result = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(result)
    return result


BASE = Path(__file__).parents[1]/'2026-10-24-provenance'
original = module('original_storage_probe', BASE/'probe.py')


class CompactTests(unittest.TestCase):
    def fixture(self, root):
        # Reuse the existing synthetic CSV builder without importing its probe alias.
        with patch.dict(sys.modules, {'probe': original}):
            fixtures = module('original_storage_tests', BASE/'test_probe.py')
        _, args = fixtures.StorageTests().fixture(root)
        with patch.object(sys, 'argv', ['probe.py', *args]), contextlib.redirect_stdout(io.StringIO()):
            original.main()
        compact_args = [str(root/'model.sqlite3'),str(root/'compact.sqlite3'),str(root/'compact.json')]
        with patch.object(sys, 'argv', ['probe.py', *compact_args]), contextlib.redirect_stdout(io.StringIO()):
            probe.main()
        return root/'model.sqlite3', root/'compact.sqlite3'

    def test_all_logical_rows_survive_compact_keys(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.fixture(root)
            result = json.loads((root/'compact.json').read_text())
            self.assertTrue(result['complete'])
            self.assertEqual(result['compared_rows']['translations'], 3)
            self.assertEqual(result['compared_rows']['import_records'], 3)
            self.assertEqual(result['compared_rows']['translation_origins'], 3)
            self.assertEqual(result['foreign_key_check'], 'ok')

    def test_comparison_detects_raw_origin_corruption(self):
        with tempfile.TemporaryDirectory() as directory:
            old_path, compact_path = self.fixture(Path(directory))
            old = probe.connect(old_path, readonly=True)
            new = probe.connect(compact_path)
            new.execute("UPDATE translation_origins SET target_text='changed' WHERE ordinal=1")
            new.commit()
            with self.assertRaisesRegex(ValueError, 'translation_origins'):
                probe.verify(old, new)
            old.close()
            new.close()

    def test_integer_artifact_keys_cannot_cross_datasets(self):
        with tempfile.TemporaryDirectory() as directory:
            _, compact_path = self.fixture(Path(directory))
            new = probe.connect(compact_path)
            new.execute('PRAGMA foreign_keys=ON')
            row = list(new.execute('SELECT * FROM datasets').fetchone())
            row[0], row[1], row[2] = 2, b'2'*16, b'3'*32
            new.execute('INSERT INTO datasets VALUES(?,?,?,?,?,?,?,?,?,?)', row)
            with self.assertRaises(sqlite3.IntegrityError):
                new.execute("INSERT INTO import_records VALUES(2,1,1,'unique',1,NULL)")
            new.rollback()
            new.close()


if __name__ == '__main__':
    unittest.main()
