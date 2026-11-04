package integrity

import (
	"crypto/sha256"
	"fmt"
	"slices"
)

// The Merkle tree follows RFC 9162 §2.1: leaves are already prefixed with
// 0x00 by [LeafOf]; an interior node is SHA-256(0x01 || left || right); the
// tree of n leaves splits at the largest power of two below n; the empty tree's
// root is SHA-256 of the empty string. Building it level by level, carrying an
// unpaired last node up unchanged, yields exactly that tree.

func node(l, r Hash) Hash {
	var b [1 + 2*sha256.Size]byte
	b[0] = 0x01
	copy(b[1:], l[:])
	copy(b[1+sha256.Size:], r[:])
	return sha256.Sum256(b[:])
}

// Tree is a window's Merkle tree, built once: its root and every inclusion
// proof are then served without rehashing (O(n) to build, O(log n) a proof).
type Tree struct {
	levels [][]Hash // levels[0] are the leaves; the last level holds the root
}

// NewTree builds the tree over a copy of leaves, in canonical order ([OrderLeaves]).
func NewTree(leaves []Hash) *Tree {
	leaves = slices.Clone(leaves)
	t := &Tree{levels: [][]Hash{leaves}}
	for level := leaves; len(level) > 1; {
		next := make([]Hash, 0, (len(level)+1)/2)
		for i := 0; i+1 < len(level); i += 2 {
			next = append(next, node(level[i], level[i+1]))
		}
		if len(level)%2 == 1 {
			next = append(next, level[len(level)-1])
		}
		t.levels = append(t.levels, next)
		level = next
	}
	return t
}

// Size is the number of leaves.
func (t *Tree) Size() uint64 { return uint64(len(t.levels[0])) }

// Root is the tree's root; the empty tree has SHA-256("").
func (t *Tree) Root() Hash {
	if t.Size() == 0 {
		return sha256.Sum256(nil)
	}
	return t.levels[len(t.levels)-1][0]
}

// Proof returns the inclusion proof of the leaf at index.
func (t *Tree) Proof(index uint64) ([]Hash, error) {
	if index >= t.Size() {
		return nil, fmt.Errorf("%w: index %d of %d", ErrLeafIndex, index, t.Size())
	}
	var proof []Hash
	i := int(index)
	for _, level := range t.levels[:len(t.levels)-1] {
		switch {
		case i%2 == 1:
			proof = append(proof, level[i-1])
		case i+1 < len(level):
			proof = append(proof, level[i+1])
		}
		i /= 2
	}
	return proof, nil
}

// MerkleRoot returns the root over leaves in canonical order.
func MerkleRoot(leaves []Hash) Hash { return NewTree(leaves).Root() }

// InclusionProof returns the audit path of leaves[index]. To prove many leaves
// of one window, build a [Tree] once instead.
func InclusionProof(leaves []Hash, index uint64) ([]Hash, error) {
	return NewTree(leaves).Proof(index)
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
