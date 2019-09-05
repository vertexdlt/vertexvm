package wasm

import (
	"encoding/binary"
	"io"
)

// ReadByte read a byte from Reader r
func ReadByte(r io.Reader) (byte, error) {
	buf := make([]byte, 1)
	_, err := r.Read(buf)
	return buf[0], err
}

// ReadBytes read n byte from Reader r
func ReadBytes(r io.Reader, n uint32) ([]byte, error) {
	bytes := make([]byte, n)
	_, err := io.ReadFull(r, bytes)
	return bytes, err
}

func readU32(r io.Reader) (uint32, error) {
	var buf [4]byte
	_, err := io.ReadFull(r, buf[:])
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(buf[:]), nil
}

func readU64(r io.Reader) (uint64, error) {
	var buf [8]byte
	_, err := io.ReadFull(r, buf[:])
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(buf[:]), nil
}
