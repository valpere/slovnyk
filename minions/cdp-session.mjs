// Shared assistant-only CDP plumbing. Connects to an existing browser only.
export function loopbackURL(value) {
  const url = new URL(value);
  if (url.protocol !== 'http:' || !['127.0.0.1', 'localhost', '[::1]'].includes(url.hostname)
      || url.username || url.password) throw Error('Expected a loopback HTTP URL without credentials');
  return url;
}

export function viewport(width, height) {
  if (![width, height].every(n => Number.isInteger(n) && n >= 200 && n <= 4096)) {
    throw Error('Viewport dimensions must be integers from 200 to 4096');
  }
  return { width, height, deviceScaleFactor: 1, mobile: false };
}

async function connection(url) {
  const socket = new WebSocket(url), pending = new Map(), listeners = new Set();
  let serial = 0;
  function close() {
    for (const p of pending.values()) { clearTimeout(p.timer); p.reject(Error('CDP disconnected')); }
    pending.clear(); socket.close();
  }
  try {
    await new Promise((resolve, reject) => {
      const timer = setTimeout(() => reject(Error('CDP connection timeout')), 5000);
      socket.onopen = () => { clearTimeout(timer); resolve(); };
      socket.onerror = () => { clearTimeout(timer); reject(Error('CDP connection failed')); };
    });
  } catch (error) { close(); throw error; }
  socket.onclose = () => {
    for (const p of pending.values()) { clearTimeout(p.timer); p.reject(Error('CDP disconnected')); }
    pending.clear();
  };
  socket.onmessage = event => {
    const message = JSON.parse(event.data);
    if (!message.id) { for (const listener of listeners) listener(message); return; }
    const p = pending.get(message.id);
    if (!p) return;
    pending.delete(message.id); clearTimeout(p.timer);
    message.error ? p.reject(Error(message.error.message)) : p.resolve(message.result);
  };
  return {
    call(method, params = {}) {
      return new Promise((resolve, reject) => {
        const id = ++serial;
        const timer = setTimeout(() => { pending.delete(id); reject(Error(method + ' timeout')); }, 15000);
        pending.set(id, { resolve, reject, timer }); socket.send(JSON.stringify({ id, method, params }));
      });
    },
    listen(listener) { listeners.add(listener); return () => listeners.delete(listener); },
    close,
  };
}

export async function isolatedPage() {
  const response = await fetch('http://127.0.0.1:9222/json/version', { signal: AbortSignal.timeout(4000) });
  if (!response.ok) throw Error('Existing CDP browser unavailable');
  const version = await response.json();
  const browser = await connection(version.webSocketDebuggerUrl);
  let browserContextId, page;
  async function close() {
    page?.close();
    try { if (browserContextId) await browser.call('Target.disposeBrowserContext', { browserContextId }); }
    finally { browser.close(); }
  }
  try {
    ({ browserContextId } = await browser.call('Target.createBrowserContext'));
    const { targetId } = await browser.call('Target.createTarget', { url: 'about:blank', browserContextId });
    const address = new URL(version.webSocketDebuggerUrl);
    address.pathname = '/devtools/page/' + targetId;
    page = await connection(address.href);
    for (const method of ['Page.enable', 'DOM.enable', 'Network.enable']) await page.call(method);
    await page.call('Emulation.setScriptExecutionDisabled', { value: true });
    return { ...page, close };
  } catch (error) { await close(); throw error; }
}

export async function navigate(page, url, width, height) {
  await page.call('Emulation.setDeviceMetricsOverride', viewport(width, height));
  const mainFrame = (await page.call('Page.getFrameTree')).frameTree.frame.id;
  let loaded = false, status;
  const off = page.listen(message => {
    if (message.method === 'Page.loadEventFired') loaded = true;
    if (message.method === 'Network.responseReceived' && message.params.type === 'Document' && message.params.frameId === mainFrame) {
      status = message.params.response.status;
    }
  });
  try {
    const result = await page.call('Page.navigate', { url: loopbackURL(url).href });
    if (result.errorText) throw Error('Page navigation failed');
    const deadline = Date.now() + 15000;
    while (!loaded && Date.now() < deadline) await new Promise(resolve => setTimeout(resolve, 50));
    if (!loaded) throw Error('Page load timeout');
    if (status !== 200) throw Error('Expected HTTP 200, received ' + status);
    await new Promise(resolve => setTimeout(resolve, 200));
    return page.call('Page.getLayoutMetrics');
  } finally { off(); }
}

export async function screenshot(page, fullPage = true) {
  const { cssContentSize: size } = await page.call('Page.getLayoutMetrics');
  if (fullPage && size.height > 20000) throw Error('Full-page screenshot exceeds 20000px');
  const options = fullPage ? { captureBeyondViewport: true, clip: { x: 0, y: 0, width: size.width, height: size.height, scale: 1 } } : { captureBeyondViewport: false };
  const result = await page.call('Page.captureScreenshot', { format: 'png', ...options });
  return Buffer.from(result.data, 'base64');
}
