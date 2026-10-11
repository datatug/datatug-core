import { readFile } from 'node:fs/promises';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { parentPort } from 'node:worker_threads';
import { dirname, join } from 'node:path';

const directory = dirname(fileURLToPath(import.meta.url));
globalThis.datatugRecordsetCompareReady = (abi) => parentPort.postMessage({ type: 'ready', abi });

async function start() {
  await import(pathToFileURL(join(directory, 'wasm_exec.js')));
  const go = new globalThis.Go();
  const bytes = await readFile(join(directory, 'recordsetcompare.wasm'));
  const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
  parentPort.on('message', (message) => {
    if (message?.type !== 'compare') return;
    try {
      parentPort.postMessage({ type: 'result', json: globalThis.datatugRecordsetCompare(message.input) });
    } catch (error) {
      parentPort.postMessage({ type: 'error', message: String(error) });
    }
  });
  void go.run(instance).catch((error) => parentPort.postMessage({ type: 'error', message: String(error) }));
}

start().catch((error) => parentPort.postMessage({ type: 'error', message: String(error) }));
