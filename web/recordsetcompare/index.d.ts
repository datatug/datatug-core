export type RecordsetCompare = {
  compareJSON(input: string): Promise<string>;
};

export declare const RECORDSET_COMPARE_ABI: 'recordsetcompare/1';
export declare function createRecordsetComparer(options: {
  workerURL?: string | URL;
  timeoutMs?: number;
  /** SHA-256 pinned by the consuming application, independent of the fetched package. */
  wasmSHA256: string;
  /** Exact byte count for the same pinned WASM file. */
  wasmBytes: number;
}): RecordsetCompare;
