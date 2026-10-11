import { Worker } from 'node:worker_threads';

const abi = 'recordsetcompare/1';
const maxInputBytes = 1 << 20;

/** Node test/diagnostic adapter. Browser consumers use index.mjs. */
export function createNodeRecordsetComparer({
  workerURL = new URL('./recordsetcompare-node-worker.mjs', import.meta.url),
  timeoutMs = 10_000,
} = {}) {
  return {
    compareJSON(input) {
      if (typeof input !== 'string') return Promise.reject(new TypeError('comparison input must be a JSON string'));
      if (Buffer.byteLength(input, 'utf8') > maxInputBytes) {
        return Promise.reject(new RangeError('comparison input exceeds the 1 MiB limit'));
      }
      const worker = new Worker(workerURL);
      return new Promise((resolve, reject) => {
        let ready = false;
        let completed = false;
        const finish = (error, value) => {
          if (completed) return;
          completed = true;
          clearTimeout(timer);
          void worker.terminate();
          if (error) reject(error);
          else resolve(value);
        };
        const timer = setTimeout(() => finish(new Error('comparison worker timed out')), timeoutMs);
        worker.on('message', (message) => {
          if (message?.type === 'error') return finish(new Error(message.message || 'comparison worker failed'));
          if (!ready) {
            if (message?.type !== 'ready' || message.abi !== abi) {
              return finish(new Error('comparison worker reported an incompatible ABI'));
            }
            ready = true;
            worker.postMessage({ type: 'compare', input });
            return;
          }
          if (message?.type !== 'result' || typeof message.json !== 'string') {
            return finish(new Error(message?.message || 'comparison worker returned an invalid response'));
          }
          finish(undefined, message.json);
        });
        worker.on('error', (error) => finish(error));
        worker.on('exit', (code) => {
          if (code !== 0 && !completed) finish(new Error(`comparison worker exited with status ${code}`));
        });
      });
    },
  };
}

export { abi as RECORDSET_COMPARE_ABI };
