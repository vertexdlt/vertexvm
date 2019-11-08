package wasm

import (
	"io"

	"github.com/vertexdlt/vertexvm/leb128"
)

type wasmReader struct {
	b      []byte
	curPos uint32
}

func (wr *wasmReader) Read(n uint32) (b []byte, err error) {
	if wr.curPos+n > uint32(len(wr.b)) {
		return []byte{}, io.EOF
	}

	b = wr.b[wr.curPos : wr.curPos+n]
	wr.curPos = wr.curPos + n
	return b, nil
}

func (wr *wasmReader) ReadOne() (b byte, err error) {
	if wr.curPos+1 > uint32(len(wr.b)) {
		return b, io.EOF
	}

	b = wr.b[wr.curPos : wr.curPos+2][0]
	wr.curPos = wr.curPos + 1
	return b, nil
}

func (wr *wasmReader) copyAll() (b []byte) {
	return wr.b[wr.curPos:len(wr.b)]
}

func (wr *wasmReader) readLeb128Uint32() (uint32, error) {
	b := wr.b[wr.curPos:len(wr.b)]
	bitcnt, res, err := leb128.ReadUint32(b)
	if err != nil {
		return 0, err
	}

	wr.curPos += bitcnt
	return res, nil
}

func (wr *wasmReader) readLeb128Int32() (int32, error) {
	b := wr.b[wr.curPos:len(wr.b)]
	bitcnt, res, err := leb128.ReadInt32(b)
	if err != nil {
		return 0, err
	}

	wr.curPos += bitcnt
	return res, nil
}

func (wr *wasmReader) readLeb128Int64() (int64, error) {
	b := wr.b[wr.curPos:len(wr.b)]
	bitcnt, res, err := leb128.ReadInt64(b)
	if err != nil {
		return 0, err
	}

	wr.curPos += bitcnt
	return res, nil
}
