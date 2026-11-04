package integrity

import (
	"crypto/sha256"
	"errors"
	"testing"
)

// naiveRoot is an independent RFC 9162 MTH implementation.
func naiveRoot(leaves []Hash) Hash {
	if len(leaves) == 0 {
		return sha256.Sum256(nil)
	}
	if len(leaves) == 1 {
		return leaves[0]
	}
	k := 1
	for k*2 < len(leaves) {
		k *= 2
	}
	l, r := naiveRoot(leaves[:k]), naiveRoot(leaves[k:])
	return sha256.Sum256(append(append([]byte{1}, l[:]...), r[:]...))
}

func leavesN(n int) []Hash {
	out := make([]Hash, n)
	for i := range out {
		out[i] = sha256.Sum256([]byte{byte(i), byte(i >> 8)})
	}
	return out
}

func TestMerkleRootAndProofs(t *testing.T) {
	if MerkleRoot(nil) != sha256.Sum256(nil) {
		t.Fatal("empty tree root")
	}
	for n := 1; n <= 70; n++ {
		leaves := leavesN(n)
		root := MerkleRoot(leaves)
		if root != naiveRoot(leaves) {
			t.Fatalf("n=%d: root differs from reference", n)
		}
		for i := range leaves {
			proof, err := InclusionProof(leaves, uint64(i))
			if err != nil {
				t.Fatal(err)
			}
			if err := VerifyInclusion(leaves[i], uint64(i), uint64(n), proof, root); err != nil {
				t.Fatalf("n=%d i=%d: %v", n, i, err)
			}
			if n > 1 {
				j := (i + 1) % n
				if VerifyInclusion(leaves[i], uint64(j), uint64(n), proof, root) == nil {
					t.Fatalf("n=%d: leaf %d verified at %d", n, i, j)
				}
				if VerifyInclusion(leaves[j], uint64(i), uint64(n), proof, root) == nil {
					t.Fatalf("n=%d: wrong leaf verified at %d", n, i)
				}
			}
		}
	}
}

func TestVerifyInclusionRejects(t *testing.T) {
	leaves := leavesN(5)
	root := MerkleRoot(leaves)
	proof, _ := InclusionProof(leaves, 2)
	cases := []struct {
		name  string
		index uint64
		size  uint64
		proof []Hash
		want  error
	}{
		{"index past size", 5, 5, proof, ErrLeafIndex},
		{"proof too long", 2, 5, append(append([]Hash{}, proof...), Hash{}, Hash{}), ErrInclusionProof},
		{"proof too short", 2, 5, proof[:1], ErrInclusionProof},
		{"smaller tree", 2, 4, proof, ErrInclusionProof},
		{"size one with path", 0, 1, proof, ErrInclusionProof},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := VerifyInclusion(leaves[2], tc.index, tc.size, tc.proof, root); !errors.Is(err, tc.want) {
				t.Fatalf("want %v, got %v", tc.want, err)
			}
		})
	}
	if _, err := InclusionProof(leaves, 5); !errors.Is(err, ErrLeafIndex) {
		t.Fatalf("want ErrLeafIndex, got %v", err)
	}
}
