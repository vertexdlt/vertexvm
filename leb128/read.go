package leb128

import (
	"errors"
)

// Read reads an unsigned integer of size n defined in https://webassembly.github.io/spec/core/binary/values.html#binary-int
// Read panics if n>64.
func Read(b []byte, maxbit uint32, hasSign bool) (uint32, int64, error) {
	if maxbit > 64 {
		return 0, 0, errors.New("leb128: n must <= 64")
	}
	var (
		shift   uint32
		bytecnt uint32
		cur     int64
		result  int64
		sign    int64 = -1
	)
	for i := 0; i < len(b); i++ {
		cur = int64(b[i])
		result |= (cur & 0x7f) << shift
		shift += 7
		sign <<= 7
		bytecnt++
		if cur&0x80 == 0 {
			break
		}
		if bytecnt > (maxbit+7-1)/7 {
			return 0, 0, errors.New("Unsigned LEB at byte overflow")
		}
	}
	if hasSign && ((sign>>1)&result) != 0 {
		result |= sign
	}
	return bytecnt, result, nil
}

// ReadUint32 reads a LEB128 encoded unsigned 32-bit integer from r, and
// returns the integer value, and the error (if any).
func ReadUint32(b []byte) (uint32, uint32, error) {
	bytecnt, result, err := Read(b, 32, false)
	return bytecnt, uint32(result), err
}

// ReadInt32 reads a LEB128 encoded signed 32-bit integer from r, and
// returns the integer value, and the error (if any).
func ReadInt32(b []byte) (uint32, int32, error) {
	bytecnt, result, err := Read(b, 32, true)
	return bytecnt, int32(result), err
}

// ReadUint64 reads a LEB128 encoded unsigned 64-bit integer from r, and
// returns the integer value, and the error (if any).
func ReadUint64(b []byte) (uint32, uint64, error) {
	bytecnt, result, err := Read(b, 64, false)
	return bytecnt, uint64(result), err
}

// ReadInt64 reads a LEB128 encoded signed 64-bit integer from r, and
// returns the integer value, and the error (if any).
func ReadInt64(b []byte) (uint32, int64, error) {
	bytecnt, result, err := Read(b, 64, true)
	return bytecnt, result, err
}
