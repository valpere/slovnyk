import test from 'node:test';
import assert from 'node:assert/strict';
import { loopbackURL, viewport } from './cdp-session.mjs';

test('CDP navigation input accepts loopback only and rejects credentials', () => {
  for (const address of ['http://127.0.0.1:8788/', 'http://localhost:8788/', 'http://[::1]:8788/']) assert(loopbackURL(address));
  for (const address of ['https://localhost/', 'http://example.org/', 'file:///tmp/test', 'http://user:secret@localhost/']) assert.throws(() => loopbackURL(address));
});
test('bounded viewport rejects malformed or unbounded dimensions', () => {
  assert.equal(viewport(320, 844).width, 320);
  for (const width of [0, 199, 4097, 390.5, NaN]) assert.throws(() => viewport(width, 844));
});
