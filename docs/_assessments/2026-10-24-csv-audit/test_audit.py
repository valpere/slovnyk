"""Synthetic boundary fixtures for the diagnostic audit."""

import gzip
import tempfile
import unittest
import zlib
from pathlib import Path

from audit import audit_file
from verify_snapshot import strict_gzip_size


class AuditTests(unittest.TestCase):
    def run_fixture(self, data=None, compressed=None):
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder) / 'slovnyk_en-us_uk-ua.csv.gz'
            path.write_bytes(gzip.compress(data) if compressed is None else compressed)
            return audit_file(path, {'en-us', 'uk-ua'})

    def test_duplicates_are_separate_from_normalization_collisions(self):
        data = ('en-us,uk-ua,Straße,вулиця\n'
                'en-us,uk-ua,STRASSE,вулиця\n'
                'en-us,uk-ua,Straße,вулиця\n').encode()
        result = self.run_fixture(data)
        self.assertTrue(result['complete'])
        self.assertEqual((result['records'], result['accepted_unique'],
                          result['duplicate_occurrences']), (3, 2, 1))
        self.assertEqual(result['extra_source_spellings_for_same_normalized_key'], 1)

    def test_embedded_crlf_quotes_bom_and_final_record_without_newline(self):
        data = b'\xef\xbb\xbfen-us,uk-ua,"a\r\nb""c",d\r\nen-us,uk-ua,e,f'
        result = self.run_fixture(data)
        self.assertTrue(result['complete'])
        self.assertTrue(result['bom'])
        self.assertEqual(result['records'], 2)
        self.assertEqual(result['issues']['text_contains_CR'], 1)
        self.assertEqual(result['max_field_codepoints'][2], 6)

    def test_malformed_quotes_and_bare_cr_are_fatal(self):
        for data in [b'en-us,uk-ua,a"b,c\n', b'en-us,uk-ua,"a"x,c\n',
                     b'en-us,uk-ua,a,b\r', b'en-us,uk-ua,"a,b\n']:
            with self.subTest(data=data):
                result = self.run_fixture(data)
                self.assertFalse(result['complete'])
                self.assertIn('fatal_error', result)

    def test_empty_physical_record_and_wrong_field_count_not_skipped(self):
        result = self.run_fixture(b'\nen-us,uk-ua,a\nen-us,uk-ua,a,b\n')
        self.assertTrue(result['complete'])
        self.assertEqual(result['records'], 3)
        self.assertEqual(result['rejected_records'], 2)

    def test_admission_uses_utf8_bytes_and_preserves_whitespace(self):
        data = ('en-us,uk-ua,' + 'ї' * 32769 + ',b\n'
                'en-us,uk-ua, a ,b\n'
                'en-us,uk-ua,,b\n'
                'en-us,uk-ua,a,\0b\n').encode()
        result = self.run_fixture(data)
        self.assertTrue(result['complete'])
        self.assertEqual(result['rejected_records'], 3)
        self.assertEqual(result['accepted_unique'], 1)
        self.assertEqual(result['issues']['source_edge_whitespace'], 1)
        self.assertEqual(result['max_field_utf8_bytes'][2], 65538)

    def test_members_utf8_and_gzip_integrity(self):
        row = b'en-us,uk-ua,a,b\n'
        result = self.run_fixture(compressed=gzip.compress(row) + gzip.compress(row))
        self.assertTrue(result['complete'])
        self.assertEqual(result['duplicate_occurrences'], 1)
        for compressed in [gzip.compress(row)[:-1], gzip.compress(row) + b'garbage',
                           gzip.compress(b'en-us,uk-ua,\xff,b\n')]:
            self.assertFalse(self.run_fixture(compressed=compressed)['complete'])

    def test_strict_member_boundaries_reject_zero_padding(self):
        member = gzip.compress(b'abc')
        self.assertEqual(strict_gzip_size(member + member), (6, 2))
        for data in [member + b'\0', member + b'garbage', member[:-1], b'']:
            with self.subTest(data=data), self.assertRaises((ValueError, zlib.error)):
                strict_gzip_size(data)


if __name__ == '__main__':
    unittest.main()
