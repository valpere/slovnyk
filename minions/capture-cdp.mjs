// One screenshot via existing Chromium, with isolated cookies and no page JS.
import { writeFile } from 'node:fs/promises';
import { loopbackURL, viewport, isolatedPage, navigate, screenshot } from './cdp-session.mjs';

const [address, output, widthArg = '1280', heightArg = '900'] = process.argv.slice(2);
const width = Number(widthArg), height = Number(heightArg);
const url = loopbackURL(address);
viewport(width, height);
if (!output) throw Error('usage: node minions/capture-cdp.mjs LOOPBACK_URL NEW_OUTPUT.png [width height]');
const page = await isolatedPage();
try {
  await navigate(page, url.href, width, height);
  await writeFile(output, await screenshot(page, false), { flag: 'wx', mode: 0o600 });
  console.log(`Saved ${output} (${width}×${height}); isolated cookies, page scripts disabled`);
} finally { await page.close(); }
