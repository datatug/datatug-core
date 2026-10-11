import assert from 'node:assert/strict';
import { pathToFileURL } from 'node:url';
import { test } from 'node:test';
import { Worker } from 'node:worker_threads';
import { createNodeRecordsetComparer } from '../node-loader.mjs';

const directory = process.env.COMPARE_PACKAGE_DIR;
if (!directory) throw new Error('COMPARE_PACKAGE_DIR must point at the built package files');

const input = JSON.stringify({
  abi: 'recordsetcompare/1',
  left: { columns: [{ name: 'id', type: 'integer' }, { name: 'value', type: 'decimal' }], rows: [[{ type: 'integer', value: '9223372036854775807' }, { type: 'decimal', value: '1.00' }]] },
  right: { columns: [{ name: 'id', type: 'integer' }, { name: 'value', type: 'decimal' }], rows: [[{ type: 'integer', value: '9223372036854775807' }, { type: 'decimal', value: '1.0' }]] },
  leftInfo: { rowCount: 1, limitations: [] },
  rightInfo: { rowCount: 1, limitations: [] },
  key: ['id'],
});

test('packaged linked WASM starts in a worker and compares exact typed text', async () => {
  const base = pathToFileURL(`${directory.replace(/\/$/, '')}/`);
  const comparer = createNodeRecordsetComparer({ workerURL: new URL('recordsetcompare-node-worker.mjs', base) });
  const response = JSON.parse(await comparer.compareJSON(input));
  assert.equal(response.abi, 'recordsetcompare/1');
  assert.equal(response.ok, true);
  assert.equal(response.diff.summary.changed, 1);
  assert.equal(response.diff.changed[0].key[0].value, '9223372036854775807');
  assert.equal(response.diff.changed[0].columns[0].left.value, '1.00');
  assert.equal(response.diff.changed[0].columns[0].right.value, '1.0');
});

test('packaged worker returns bounded adapter errors without terminating the process', async () => {
  const base = pathToFileURL(`${directory.replace(/\/$/, '')}/`);
  const comparer = createNodeRecordsetComparer({ workerURL: new URL('recordsetcompare-node-worker.mjs', base) });
  const response = JSON.parse(await comparer.compareJSON(input.replace('recordsetcompare/1', 'recordsetcompare/2')));
  assert.equal(response.ok, false);
  assert.equal(response.error.code, 'invalid_abi');
});

test('linked Go callback rejects a non-string argument through the real worker bridge', async () => {
  const base = pathToFileURL(`${directory.replace(/\/$/, '')}/`);
  const worker = new Worker(new URL('recordsetcompare-node-worker.mjs', base));
  try {
    const response = await new Promise((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error('direct callback test timed out')), 10_000);
      worker.on('error', reject);
      worker.on('message', (message) => {
        if (message.type === 'error') {
          clearTimeout(timer);
          reject(new Error(message.message));
        } else if (message.type === 'ready') {
          worker.postMessage({ type: 'compare', input: 42 });
        } else if (message.type === 'result') {
          clearTimeout(timer);
          resolve(JSON.parse(message.json));
        }
      });
    });
    assert.equal(response.ok, false);
    assert.equal(response.error.code, 'invalid_request');
  } finally {
    await worker.terminate();
  }
});
