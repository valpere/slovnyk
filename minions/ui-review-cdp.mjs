// Promote repeated scratch UI review matrices into one assistant-only command.
import { readFile, writeFile, mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { loopbackURL, viewport, isolatedPage, navigate, screenshot } from './cdp-session.mjs';

const [address, configPath, output] = process.argv.slice(2);
if (!address || !configPath || !output) throw Error('usage: node minions/ui-review-cdp.mjs LOOPBACK_BASE CONFIG.json NEW_OUTPUT_DIRECTORY');
const base = loopbackURL(address);
if (base.pathname !== '/' || base.search || base.hash) throw Error('Base must be an origin');
const config = JSON.parse(await readFile(configPath, 'utf8'));
const widths = config.widths ?? [320, 390, 1280], height = config.height ?? 900;
if (!Array.isArray(widths) || !widths.length || widths.length > 10) throw Error('Expected 1–10 widths');
for (const width of widths) viewport(width, height);
if (!Array.isArray(config.cases) || !config.cases.length || config.cases.length > 200) throw Error('Expected 1–200 cases');
const names = new Set();
for (const c of config.cases) {
  if (typeof c.name !== 'string' || !/^[a-z0-9][a-z0-9-]{0,63}$/.test(c.name) || names.has(c.name)) throw Error('Case names must be unique safe filename labels');
  names.add(c.name);
  if (typeof c.path !== 'string' || !c.path.startsWith('/') || new URL(c.path, base).origin !== base.origin) throw Error('Case must stay on base origin');
  if (typeof c.lang !== 'string' || !c.lang) throw Error('Every case requires expected html lang');
}
await mkdir(output, { mode: 0o700 }); // Refuse to overwrite earlier evidence.
const results = [];
let page, failure;
try {
  page = await isolatedPage();
  if (config.accept_language) await page.call('Network.setExtraHTTPHeaders', { headers: { 'Accept-Language': config.accept_language } });
  for (const c of config.cases) {
    for (const width of widths) {
      const result = { name: c.name, width, height, passed: false };
      results.push(result);
      try {
        const layout = await navigate(page, new URL(c.path, base).href, width, height);
        result.content_width = layout.cssContentSize.width;
        if (result.content_width > width + 1) throw Error('Horizontal overflow');
        const doc = (await page.call('DOM.getDocument')).root.nodeId;
        const { nodeId } = await page.call('DOM.querySelector', { nodeId: doc, selector: 'html' });
        const { attributes } = await page.call('DOM.getAttributes', { nodeId });
        const attrs = Object.fromEntries(Array.from({ length: attributes.length / 2 }, (_, i) => [attributes[2*i], attributes[2*i+1]]));
        result.lang = attrs.lang;
        if (attrs.lang !== c.lang) throw Error('Unexpected html lang');
        if (c.screenshot === true) await writeFile(join(output, `${c.name}-${width}.png`), await screenshot(page), { flag: 'wx', mode: 0o600 });
        result.passed = true;
      } catch (error) { result.error = error.message; throw error; }
    }
  }
} catch (error) { failure = error; }
finally {
  try { if (page) await page.close(); }
  catch (error) { failure = failure ? Error(failure.message + '; cleanup: ' + error.message) : error; }
  await writeFile(join(output, 'results.json'), JSON.stringify({ passed: !failure, cases: results, error: failure?.message }, null, 2) + '\n', { flag: 'wx', mode: 0o600 });
}
if (failure) throw failure;
console.log(`PASS ${results.length} viewports: HTTP 200, expected lang, no horizontal overflow; isolated cookies, page scripts disabled`);
