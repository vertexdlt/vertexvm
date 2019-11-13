package util

import (
	"io"
)

type ByteReader struct {
	b      []byte
	curPos uint32
}

func NewByteReader(b []byte) *ByteReader {
	return &ByteReader{b, 0}
}

func (wr *ByteReader) Read(n uint32) (b []byte, err error) {
	if wr.curPos+n > uint32(len(wr.b)) {
		return []byte{}, io.EOF
	}

	b = wr.b[wr.curPos : wr.curPos+n]
	wr.curPos = wr.curPos + n
	return b, nil
}

func (wr *ByteReader) ReadOne() (b byte, err error) {
	if wr.curPos+1 > uint32(len(wr.b)) {
		return b, io.EOF
	}

	b = wr.b[wr.curPos : wr.curPos+2][0]
	wr.curPos = wr.curPos + 1
	return b, nil
}

func (wr *ByteReader) CopyAll() (b []byte) {
	return wr.b[wr.curPos:len(wr.b)]
}
