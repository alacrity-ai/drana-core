package types

import (
	"crypto/sha256"
	"encoding/binary"
)

// HashWriter accumulates data for deterministic hashing.
// Fields must always be written in a fixed order with length prefixes
// to avoid ambiguity.
type HashWriter struct {
	buf []byte
}

func NewHashWriter() *HashWriter {
	return &HashWriter{}
}

func (hw *HashWriter) WriteBytes(b []byte) {
	// Length-prefix to prevent concatenation ambiguity.
	var lenBuf [8]byte
	binary.BigEndian.PutUint64(lenBuf[:], uint64(len(b)))
	hw.buf = append(hw.buf, lenBuf[:]...)
	hw.buf = append(hw.buf, b...)
}

func (hw *HashWriter) WriteUint64(v uint64) {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], v)
	hw.buf = append(hw.buf, buf[:]...)
}

func (hw *HashWriter) WriteUint32(v uint32) {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], v)
	hw.buf = append(hw.buf, buf[:]...)
}

func (hw *HashWriter) WriteInt64(v int64) {
	hw.WriteUint64(uint64(v))
}

func (hw *HashWriter) WriteString(s string) {
	hw.WriteBytes([]byte(s))
}

func (hw *HashWriter) Sum256() [32]byte {
	return sha256.Sum256(hw.buf)
}
