// Package importdata implements the lossless CSV4 and manifest V1 formats.
package importdata

import (
	"crypto/sha256"
	"encoding/binary"
	"hash"
)

const ParserVersion = "lossless-csv4-v1"
const MappingVersion = "identity-v1"

type Tuple [4]string

// WriteTuple appends the canonical uint64-BE length-prefixed four-field tuple.
// Use a streaming hash over sorted tuples for per-pair verification digests.
func WriteTuple(h hash.Hash, t Tuple) {
	var length [8]byte
	for _, s := range t {
		binary.BigEndian.PutUint64(length[:], uint64(len(s)))
		h.Write(length[:])
		h.Write([]byte(s))
	}
}

func TupleHash(t Tuple) [32]byte {
	h := sha256.New()
	WriteTuple(h, t)
	return [32]byte(h.Sum(nil))
}
