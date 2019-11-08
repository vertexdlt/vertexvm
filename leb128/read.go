package leb128

import (
	"errors"
	"log"
)

// Read reads an unsigned integer of size n defined in https://webassembly.github.io/spec/core/binary/values.html#binary-int
// Read panics if n>64.
func Read(b []byte, n uint32, hasSign bool) (uint32, int64, error) {
	if n > 64 {
		panic(errors.New("leb128: n must <= 64"))
	}
	var (
		shift  uint32
		bitcnt uint32
		cur    int64
		result int64
		sign   int64 = -1
	)
	for i := 0; i < len(b); i++ {
		cur = int64(b[i])
		result |= (cur & 0x7f) << shift
		shift += 7
		sign <<= 7
		bitcnt++
		if cur&0x80 == 0 {
			break
		}
		if bitcnt > (n+7-1)/7 {
			log.Fatal("Unsigned LEB at byte overflow")
		}
	}
	if hasSign && ((sign>>1)&result) != 0 {
		result |= sign
	}
	return bitcnt, result, nil
}

// ReadUint32 reads a LEB128 encoded unsigned 32-bit integer from r, and
// returns the integer value, and the error (if any).
func ReadUint32(b []byte) (uint32, uint32, error) {
	bitcnt, result, err := Read(b, 32, false)
	return bitcnt, uint32(result), err
}

// ReadInt32 reads a LEB128 encoded signed 32-bit integer from r, and
// returns the integer value, and the error (if any).
func ReadInt32(b []byte) (uint32, int32, error) {
	bitcnt, result, err := Read(b, 32, true)
	return bitcnt, int32(result), err
}

// ReadUint64 reads a LEB128 encoded unsigned 64-bit integer from r, and
// returns the integer value, and the error (if any).
func ReadUint64(b []byte) (uint32, uint64, error) {
	bitcnt, result, err := Read(b, 64, false)
	return bitcnt, uint64(result), err
}

// ReadInt64 reads a LEB128 encoded signed 64-bit integer from r, and
// returns the integer value, and the error (if any).
func ReadInt64(b []byte) (uint32, int64, error) {
	bitcnt, result, err := Read(b, 64, true)
	return bitcnt, result, err
}
