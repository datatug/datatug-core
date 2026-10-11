import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { extname, join, normalize } from 'node:path';
import { createHash } from 'node:crypto';
import { test, expect } from '@playwright/test';

const packageDirectory = process.env.COMPARE_PACKAGE_DIR;
if (!packageDirectory) throw new Error('COMPARE_PACKAGE_DIR must point at the built package files');
const wasmFile = await readFile(join(packageDirectory, 'recordsetcompare.wasm'));
const wasmPin = {
  wasmBytes: wasmFile.byteLength,
  wasmSHA256: createHash('sha256').update(wasmFile).digest('hex'),
};

const input = JSON.stringify({
  abi: 'recordsetcompare/1',
  left: { columns: [{ name: 'id', type: 'integer' }, { name: 'value', type: 'decimal' }], rows: [[{ type: 'integer', value: '9223372036854775807' }, { type: 'decimal', value: '1.00' }]] },
  right: { columns: [{ name: 'id', type: 'integer' }, { name: 'value', type: 'decimal' }], rows: [[{ type: 'integer', value: '9223372036854775807' }, { type: 'decimal', value: '1.0' }]] },
  leftInfo: { rowCount: 1, limitations: [] },
  rightInfo: { rowCount: 1, limitations: [] },
  key: ['id'],
});

let server;
let origin;
test.beforeAll(async () => {
  server = createServer(async (request, response) => {
    const path = request.url === '/' ? '/test.html' : decodeURIComponent(request.url.split('?')[0]);
    if (path === '/test.html') {
      response.writeHead(200, { 'content-type': 'text/html', ...csp });
      response.end('<!doctype html><meta charset="utf-8"><script type="module" src="/harness.mjs"></script><div id="ready">ready</div>');
      return;
    }
    if (path === '/harness.mjs') {
      response.writeHead(200, { 'content-type': 'text/javascript', ...csp });
      response.end(`import {createRecordsetComparer} from '/index.mjs'; window.wasmPin = ${JSON.stringify(wasmPin)}; window.compare = (input) => createRecordsetComparer(window.wasmPin).compareJSON(input); window.compareWithPin = (input, pin) => createRecordsetComparer(pin).compareJSON(input); window.compareWithWorker = (input, workerURL, timeoutMs) => createRecordsetComparer({...window.wasmPin, workerURL, timeoutMs}).compareJSON(input);`);
      return;
    }
    if (path === '/hang.js') {
      response.writeHead(200, { 'content-type': 'text/javascript', ...csp });
      response.end('setInterval(() => {}, 1000);');
      return;
    }
    if (path === '/bad-worker.js') {
      response.writeHead(200, { 'content-type': 'text/javascript', ...csp });
      response.end('throw new Error("intentional worker startup failure");');
      return;
    }
    const filename = normalize(join(packageDirectory, path.slice(1)));
    if (!filename.startsWith(normalize(packageDirectory))) {
      response.writeHead(404); response.end(); return;
    }
    try {
      const file = await readFile(filename);
      response.writeHead(200, { 'content-type': contentType(extname(filename)), ...csp, 'content-length': file.length });
      response.end(file);
    } catch {
      response.writeHead(404); response.end();
    }
  });
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  origin = `http://127.0.0.1:${server.address().port}`;
});

test.afterAll(async () => {
  if (!server?.listening) return;
  await new Promise((resolve, reject) => server.close((error) => error ? reject(error) : resolve()));
});

const csp = { 'content-security-policy': "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; worker-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'" };
function contentType(extension) {
  return ({ '.mjs': 'text/javascript', '.js': 'text/javascript', '.wasm': 'application/wasm' })[extension] || 'application/octet-stream';
}

test('real Chromium loads packaged Go WASM under same-origin CSP and keeps integer/decimal text exact', async ({ page }) => {
  await page.goto(`${origin}/`);
  await expect(page.locator('#ready')).toHaveText('ready');
  const response = JSON.parse(await page.evaluate((value) => window.compare(value), input));
  expect(response.ok).toBe(true);
  expect(response.diff.summary.changed).toBe(1);
  expect(response.diff.changed[0].key[0].value).toBe('9223372036854775807');
  expect(response.diff.changed[0].columns[0].left.value).toBe('1.00');
  expect(response.diff.changed[0].columns[0].right.value).toBe('1.0');
});

test('browser worker startup errors and timeouts reject and terminate cleanly', async ({ page }) => {
  await page.goto(`${origin}/`);
  const failure = await page.evaluate(async () => {
    try {
      await window.compareWithWorker('{}', '/hang.js', 50);
      return '';
    } catch (error) {
      return error.message;
    }
  });
  expect(failure).toBe('comparison worker timed out');

  const startupFailure = await page.evaluate(async () => {
    try {
      await window.compareWithWorker('{}', '/bad-worker.js', 2_000);
      return '';
    } catch (error) {
      return error.message;
    }
  });
  expect(startupFailure).toContain('intentional worker startup failure');
});

test('browser worker rejects bytes that fail the consumer-supplied external pin', async ({ page }) => {
  await page.goto(`${origin}/`);
  const failure = await page.evaluate(async (value) => {
    try {
      await window.compareWithPin(value, { ...window.wasmPin, wasmSHA256: '0'.repeat(64) });
      return '';
    } catch (error) {
      return error.message;
    }
  }, input);
  expect(failure).toContain('SHA-256 does not match its external pin');
});

test('actual linked callback returns a strict error for a non-string worker argument', async ({ page }) => {
  await page.goto(`${origin}/`);
  const response = await page.evaluate(async () => new Promise((resolve, reject) => {
    const worker = new Worker('/recordsetcompare-worker.js');
    const timer = setTimeout(() => {
      worker.terminate();
      reject(new Error('direct callback test timed out'));
    }, 10_000);
    worker.onmessage = ({ data }) => {
      if (data.type === 'ready') worker.postMessage({ type: 'compare', input: 42 });
      else if (data.type === 'result') {
        clearTimeout(timer);
        worker.terminate();
        resolve(JSON.parse(data.json));
      } else if (data.type === 'error') {
        clearTimeout(timer);
        worker.terminate();
        reject(new Error(data.message));
      }
    };
    worker.postMessage({ type: 'initialize', abi: 'recordsetcompare/1', ...window.wasmPin });
  }));
  expect(response.ok).toBe(false);
  expect(response.error.code).toBe('invalid_request');
});
