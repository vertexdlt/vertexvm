package vm

import (
	"encoding/binary"
	"log"

	"github.com/vertexdlt/vertexvm/leb128"
	"github.com/vertexdlt/vertexvm/wasm"
)

// Frame or call frame holds the relevant execution information of a function
type Frame struct {
	fn             *wasm.Function
	ip             int
	basePointer    int
	baseBlockIndex int
}

// NewFrame initialize a call frame for a given function fn
func NewFrame(fn *wasm.Function, basePointer int, baseBlockIndex int) *Frame {
	f := &Frame{
		fn:             fn,
		ip:             -1,
		basePointer:    basePointer,
		baseBlockIndex: baseBlockIndex,
	}
	return f
}

func (frame *Frame) readLEB(maxbit uint32, hasSign bool) int64 {
	ins := frame.instructions()
	bitcnt, result, err := leb128.Read(ins[frame.ip+1:len(ins)], maxbit, hasSign)
	if err != nil {
		log.Fatal(err)
	}

	frame.ip += int(bitcnt)
	return result
}

func (frame *Frame) instructions() []byte {
	return frame.fn.Code.Exprs
}

func (frame *Frame) hasEnded() bool {
	return frame.ip == len(frame.instructions())-1
}

func (frame *Frame) readUint32() uint32 {
	data := frame.instructions()[frame.ip+1 : frame.ip+5]
	frame.ip += 4
	return binary.LittleEndian.Uint32(data)
}

func (frame *Frame) readUint64() uint64 {
	data := frame.instructions()[frame.ip+1 : frame.ip+9]
	frame.ip += 8
	return binary.LittleEndian.Uint64(data)
}
