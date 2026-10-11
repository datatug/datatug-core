const abi = 'recordsetcompare/1';
const maxInputBytes = 1 << 20;
const maxWasmBytes = 16 << 20;

/** Create a browser-only comparer. Every call receives a fresh worker isolate. */
export function createRecordsetComparer({
  workerURL = new URL('./recordsetcompare-worker.js', import.meta.url),
  timeoutMs = 10_000,
  wasmSHA256,
  wasmBytes,
} = {}) {
  if (!/^[0-9a-f]{64}$/.test(wasmSHA256 || '')) {
    throw new TypeError('wasmSHA256 must be the externally pinned lowercase SHA-256 digest');
  }
  if (!Number.isSafeInteger(wasmBytes) || wasmBytes < 1 || wasmBytes > maxWasmBytes) {
    throw new RangeError(`wasmBytes must be between 1 and ${maxWasmBytes}`);
  }
  return {
    compareJSON(input) {
      if (typeof input !== 'string') {
        return Promise.reject(new TypeError('comparison input must be a JSON string'));
      }
      if (new TextEncoder().encode(input).byteLength > maxInputBytes) {
        return Promise.reject(new RangeError('comparison input exceeds the 1 MiB limit'));
      }
      const worker = new Worker(workerURL);
      worker.postMessage({ type: 'initialize', abi, wasmSHA256, wasmBytes });
      return new Promise((resolve, reject) => {
        let ready = false;
        let completed = false;
        const timer = setTimeout(() => finish(new Error('comparison worker timed out')), timeoutMs);
        const finish = (error, value) => {
          if (completed) return;
          completed = true;
          clearTimeout(timer);
          worker.terminate();
          if (error) reject(error);
          else resolve(value);
        };
        worker.onmessage = ({ data }) => {
          if (data?.type === 'error') {
            finish(new Error(data.message || 'comparison worker failed'));
            return;
          }
          if (!ready) {
            if (data?.type !== 'ready' || data.abi !== abi) {
              finish(new Error('comparison worker reported an incompatible ABI'));
              return;
            }
            ready = true;
            worker.postMessage({ type: 'compare', input });
            return;
          }
          if (data?.type !== 'result' || typeof data.json !== 'string') {
            finish(new Error('comparison worker returned an invalid response'));
            return;
          }
          finish(undefined, data.json);
        };
        worker.onerror = (event) => finish(event.error || new Error(event.message || 'comparison worker failed'));
        worker.onmessageerror = () => finish(new Error('comparison worker message could not be decoded'));
      });
    },
  };
}

export { abi as RECORDSET_COMPARE_ABI };
