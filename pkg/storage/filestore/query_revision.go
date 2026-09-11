package filestore

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// computeQueryRevision derives a datatug.QueryRevision from the exact bytes
// a revisioned query pair persists: the JSON metadata file's bytes, the
// body sidecar's type-derived extension (queryBodyFileExt; empty when the
// query has no body sidecar - the "no body sidecar exists" sentinel, see
// currentQueryPair.revision) and its exact bytes. Every component is
// framed with its length before being hashed, so two different (json,
// ext, body) triples can never hash the same by having bytes shift across
// a component boundary (see
// TestComputeQueryRevision_FramingPreventsBoundaryAmbiguity). An external
// hand edit of either persisted file, including a query type change that
// renames the body sidecar, therefore always produces a different revision
// and invalidates a writer holding the old one.
//
// It deliberately never hashes the sidecar's full file name: its "<id>"
// prefix is not persisted content, only the spelling a caller used to find
// the pair. On a file system that resolves several spellings of one name to
// the same file - letter case on the default macOS and Windows volumes,
// Unicode NFC/NFD on macOS - hashing it gave the very same bytes a
// different revision per spelling, and a caller that reloaded a record via
// another spelling a spurious QueryRevisionConflictError on its next
// conditional write or delete. The record's own id is still covered: the
// JSON metadata persists it. See
// TestLoadQueryRevision_FileNameAliasesOfOneRecordShareItsRevision.
func computeQueryRevision(jsonBytes []byte, bodyExt string, bodyBytes []byte) datatug.QueryRevision {
	h := sha256.New()
	writeFramed(h, jsonBytes)
	writeFramed(h, []byte(bodyExt))
	writeFramed(h, bodyBytes)
	return datatug.QueryRevision(hex.EncodeToString(h.Sum(nil)))
}

// writeFramed writes b to h prefixed by its length as a fixed-width
// big-endian uint64, so hashing never depends on how surrounding
// components happen to be delimited.
func writeFramed(h hash.Hash, b []byte) {
	var lenBuf [8]byte
	binary.BigEndian.PutUint64(lenBuf[:], uint64(len(b)))
	h.Write(lenBuf[:])
	h.Write(b)
}
