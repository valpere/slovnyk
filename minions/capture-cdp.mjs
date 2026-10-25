// Assistant-only screenshots via an already running Chromium. Never launches it.
import { writeFile } from 'node:fs/promises';

const [address, output, widthArg = '1280', heightArg = '900'] = process.argv.slice(2);
const width = Number(widthArg), height = Number(heightArg);
const page = new URL(address);
if (page.protocol !== 'http:' || !['127.0.0.1', 'localhost', '[::1]'].includes(page.hostname)
    || !output || !Number.isInteger(width) || !Number.isInteger(height)
    || width < 200 || width > 4096 || height < 200 || height > 4096) {
  throw Error('usage: node minions/capture-cdp.mjs <loopback-http-url> <output.png> [width height]');
}
const base = 'http://127.0.0.1:9222';
let target, socket;
try {
  const response = await fetch(`${base}/json/new?about:blank`, { method: 'PUT', signal: AbortSignal.timeout(4000) });
  if (!response.ok) throw Error('Cannot create CDP tab');
  target = await response.json();
  socket = new WebSocket(target.webSocketDebuggerUrl);
  await new Promise((resolve, reject) => {
    const timeout = setTimeout(() => reject(Error('CDP connection timeout')), 4000);
    socket.onopen = () => { clearTimeout(timeout); resolve(); };
    socket.onerror = () => { clearTimeout(timeout); reject(Error('CDP connection failed')); };
  });
  let serial = 0;
  const pending = new Map();
  socket.onmessage = event => {
    const message = JSON.parse(event.data), request = pending.get(message.id);
    if (!request) return;
    pending.delete(message.id);
    clearTimeout(request.timeout);
    message.error ? request.reject(Error(message.error.message)) : request.resolve(message.result);
  };
  const call = (method, params = {}) => new Promise((resolve, reject) => {
    const id = ++serial;
    const timeout = setTimeout(() => { pending.delete(id); reject(Error(`${method} timeout`)); }, 4000);
    pending.set(id, { resolve, reject, timeout });
    socket.send(JSON.stringify({ id, method, params }));
  });
  await call('Page.enable');
  await call('Page.bringToFront');
  await call('Emulation.setScriptExecutionDisabled', { value: true });
  await call('Emulation.setDeviceMetricsOverride', { width, height, deviceScaleFactor: 1, mobile: width < 600 });
  const navigation = await call('Page.navigate', { url: page.href });
  if (navigation.errorText) throw Error(navigation.errorText);
  await new Promise(resolve => setTimeout(resolve, 500));
  const screenshot = await call('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false, fromSurface: false });
  await writeFile(output, Buffer.from(screenshot.data, 'base64'));
  console.log(`Saved ${output} (${width}×${height}); page scripts disabled`);
} finally {
  socket?.close();
  if (target) {
    const response = await fetch(`${base}/json/close/${target.id}`, { signal: AbortSignal.timeout(4000) });
    if (!response.ok) throw Error('Cannot close the assistant CDP tab');
  }
}
