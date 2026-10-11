/* Classic worker: the Go runtime and module are loaded inside this isolate. */
importScripts('./wasm_exec.js');

const abi = 'recordsetcompare/1';
const maxWasmBytes = 16 << 20;
let resolveConfiguration;
let rejectConfiguration;
const configuration = new Promise((resolve, reject) => {
  resolveConfiguration = resolve;
  rejectConfiguration = reject;
});

self.addEventListener('message', (event) => {
  if (event.data?.type !== 'initialize') return;
  const { wasmSHA256, wasmBytes, abi: requestedABI } = event.data;
  if (requestedABI !== abi || !/^[0-9a-f]{64}$/.test(wasmSHA256 || '') ||
      !Number.isSafeInteger(wasmBytes) || wasmBytes < 1 || wasmBytes > maxWasmBytes) {
    rejectConfiguration(new Error('invalid externally pinned WebAssembly integrity configuration'));
    return;
  }
  resolveConfiguration({ wasmSHA256, wasmBytes });
});

async function readBounded(response, expectedBytes) {
  if (!response.body?.getReader) throw new Error('WebAssembly response does not support bounded streaming');
  const reader = response.body.getReader();
  const chunks = [];
  let size = 0;
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      size += value.byteLength;
      if (size > expectedBytes || size > maxWasmBytes) {
        await reader.cancel();
        throw new Error('WebAssembly response exceeds its externally pinned byte count');
      }
      chunks.push(value);
    }
  } finally {
    reader.releaseLock();
  }
  if (size !== expectedBytes) throw new Error('WebAssembly response byte count does not match its external pin');
  const bytes = new Uint8Array(size);
  let offset = 0;
  for (const chunk of chunks) {
    bytes.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return bytes;
}

async function start() {
  const integrity = await configuration;
  const go = new Go();
  let resolveReady;
  let rejectReady;
  const ready = new Promise((resolve, reject) => {
    resolveReady = resolve;
    rejectReady = reject;
  });
  self.datatugRecordsetCompareReady = resolveReady;
  const wasmURL = new URL('./recordsetcompare.wasm', self.location.href);
  const response = await fetch(wasmURL);
  if (!response.ok) throw new Error(`WebAssembly fetch failed: ${response.status}`);
  const bytes = await readBounded(response, integrity.wasmBytes);
  const actualSHA256 = Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256', bytes)),
    (byte) => byte.toString(16).padStart(2, '0')).join('');
  if (actualSHA256 !== integrity.wasmSHA256) {
    throw new Error('WebAssembly SHA-256 does not match its external pin');
  }
  const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
  void go.run(instance).catch((error) => {
    rejectReady(error);
    self.postMessage({ type: 'error', message: String(error) });
  });
  const reportedABI = await ready;
  if (reportedABI !== abi || typeof self.datatugRecordsetCompare !== 'function') {
    throw new Error('WebAssembly module reported an incompatible ABI');
  }
  self.postMessage({ type: 'ready', abi });
  self.addEventListener('message', (event) => {
    if (event.data?.type !== 'compare') return;
    self.postMessage({ type: 'result', json: self.datatugRecordsetCompare(event.data.input) });
  });
}

start().catch((error) => self.postMessage({ type: 'error', message: String(error) }));
