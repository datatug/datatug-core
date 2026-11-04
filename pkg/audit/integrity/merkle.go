package integrity

import (
	"crypto/sha256"
	"fmt"
	"math/bits"
)

// The Merkle tree follows RFC 9162 §2.1: leaves are already prefixed with
// 0x00 by [LeafOf]; an interior node is SHA-256(0x01 || left || right); the
// tree of n leaves splits at the largest power of two below n; the empty tree's
// root is SHA-256 of the empty string.

func node(l, r Hash) Hash {
	var b [1 + 2*sha256.Size]byte
	b[0] = 0x01
	copy(b[1:], l[:])
	copy(b[1+sha256.Size:], r[:])
	return sha256.Sum256(b[:])
}

// split returns the largest power of two strictly below n (n ≥ 2).
func split(n int) int { return 1 << (bits.Len(uint(n-1)) - 1) }

// MerkleRoot returns the root over leaves in insertion order.
func MerkleRoot(leaves []Hash) Hash {
	switch len(leaves) {
	case 0:
		return sha256.Sum256(nil)
	case 1:
		return leaves[0]
	}
	k := split(len(leaves))
	return node(MerkleRoot(leaves[:k]), MerkleRoot(leaves[k:]))
}

// InclusionProof returns the audit path of leaves[index].
func InclusionProof(leaves []Hash, index uint64) ([]Hash, error) {
	if index >= uint64(len(leaves)) {
		return nil, fmt.Errorf("%w: index %d of %d", ErrLeafIndex, index, len(leaves))
	}
	return path(leaves, int(index)), nil
}

func path(leaves []Hash, m int) []Hash {
	if len(leaves) == 1 {
		return nil
	}
	k := split(len(leaves))
	if m < k {
		return append(path(leaves[:k], m), MerkleRoot(leaves[k:]))
	}
	return append(path(leaves[k:], m-k), MerkleRoot(leaves[:k]))
}

// VerifyInclusion checks that leaf sits at index in the tree of size leaves
// whose root is root (RFC 9162 §2.1.3.2).
func VerifyInclusion(leaf Hash, index, size uint64, proof []Hash, root Hash) error {
	if index >= size {
		return fmt.Errorf("%w: index %d of %d", ErrLeafIndex, index, size)
	}
	fn, sn, r := index, size-1, leaf
	for _, p := range proof {
		if sn == 0 {
			return fmt.Errorf("%w: proof too long", ErrInclusionProof)
		}
		if fn&1 == 1 || fn == sn {
			r = node(p, r)
			for fn&1 == 0 && fn != 0 {
				fn >>= 1
				sn >>= 1
			}
		} else {
			r = node(r, p)
		}
		fn >>= 1
		sn >>= 1
	}
	if sn != 0 || r != root {
		return ErrInclusionProof
	}
	return nil
}
