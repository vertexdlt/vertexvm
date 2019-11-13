package leb128

import (
	"errors"
	"log"

	"github.com/vertexdlt/vertexvm/util"
)

// Read reads an unsigned integer of size n defined in https://webassembly.github.io/spec/core/binary/values.html#binary-int
// Read panics if n>64.
func Read(br *util.ByteReader, n uint32, hasSign bool) (int64, error) {
	if n > 64 {
		panic(errors.New("leb128: n must <= 64"))
	}
	var (
		shift   uint32
		bytecnt uint32
		cur     int64
		result  int64
		sign    int64 = -1
	)
	for {
		b, err := br.ReadOne()
		if err != nil {
			return result, err
		}
		cur = int64(b)
		result |= (cur & 0x7f) << shift
		shift += 7
		sign <<= 7
		bytecnt++
		if cur&0x80 == 0 {
			break
		}
		if bytecnt > (n+7-1)/7 {
			log.Fatal("Unsigned LEB at byte overflow")
		}
	}
	if hasSign && ((sign>>1)&result) != 0 {
		result |= sign
	}
	return result, nil
}

// ReadUint32 reads a LEB128 encoded unsigned 32-bit integer from r, and
// returns the integer value, and the error (if any).
func ReadUint32(br *util.ByteReader) (uint32, error) {
	result, err := Read(br, 32, false)
	return uint32(result), err
}

// ReadInt32 reads a LEB128 encoded signed 32-bit integer from r, and
// returns the integer value, and the error (if any).
func ReadInt32(br *util.ByteReader) (int32, error) {
	result, err := Read(br, 32, true)
	return int32(result), err
}

// ReadUint64 reads a LEB128 encoded unsigned 64-bit integer from r, and
// returns the integer value, and the error (if any).
func ReadUint64(br *util.ByteReader) (uint64, error) {
	result, err := Read(br, 64, false)
	return uint64(result), err
}

// ReadInt64 reads a LEB128 encoded signed 64-bit integer from r, and
// returns the integer value, and the error (if any).
func ReadInt64(br *util.ByteReader) (int64, error) {
	result, err := Read(br, 64, true)
	return result, err
}
