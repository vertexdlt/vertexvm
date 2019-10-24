package leb128

import (
	"io"
	"log"
)

// Read reads a LEB128 encoded integer from reader, specified by maxbit and hasSign
func Read(r io.Reader, maxbit uint32, hasSign bool) (uint32, int64, error) {
	var (
		shift  uint32
		bitcnt uint32
		cur    int64
		result int64
		sign   int64 = -1
	)

	p := make([]byte, 1)
	for {
		_, err := io.ReadFull(r, p)
		if err != nil {
			return 0, 0, err
		}
		cur = int64(p[0])
		result |= (cur & 0x7f) << shift
		shift += 7
		sign <<= 7
		bitcnt++
		if cur&0x80 == 0 {
			break
		}
		if bitcnt > (maxbit+7-1)/7 {
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
func ReadUint32(r io.Reader) (uint32, error) {
	sign := false
	_, result, err := Read(r, 32, sign)
	return uint32(result), err
}

// ReadInt32 reads a LEB128 encoded signed 32-bit integer from r, and
// returns the integer value, and the error (if any).
func ReadInt32(r io.Reader) (int32, error) {
	sign := true
	_, result, err := Read(r, 32, sign)
	return int32(result), err
}

// ReadUint64 reads a LEB128 encoded unsigned 64-bit integer from r, and
// returns the integer value, and the error (if any).
func ReadUint64(r io.Reader) (uint64, error) {
	sign := false
	_, result, err := Read(r, 64, sign)
	return uint64(result), err
}

// ReadInt64 reads a LEB128 encoded signed 64-bit integer from r, and
// returns the integer value, and the error (if any).
func ReadInt64(r io.Reader) (int64, error) {
	sign := true
	_, result, err := Read(r, 64, sign)
	return int64(result), err
}
