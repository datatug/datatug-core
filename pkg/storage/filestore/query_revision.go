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
// body sidecar's file name (empty when the query has no body sidecar - see
// noBodyFileName) and its exact bytes. Every component is framed with its
// length before being hashed, so two different (json, name, body) triples
// can never hash the same by having bytes shift across a component
// boundary (see TestComputeQueryRevision_FramingPreventsBoundaryAmbiguity).
// An external hand edit of either persisted file, including one that only
// renames the body sidecar (a query type change), therefore always
// produces a different revision and invalidates a writer holding the old
// one.
func computeQueryRevision(jsonBytes []byte, bodyFileName string, bodyBytes []byte) datatug.QueryRevision {
	h := sha256.New()
	writeFramed(h, jsonBytes)
	writeFramed(h, []byte(bodyFileName))
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
