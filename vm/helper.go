package vm

import (
	"math"

	"github.com/go-interpreter/wagon/wasm"
)

func castReturnValue(retVal uint64, retType wasm.ValueType) uint64 {
	var castVal uint64
	switch retType {
	case wasm.ValueTypeI32:
		castVal = uint64(int32(retVal))
	case wasm.ValueTypeI64:
		castVal = retVal
	case wasm.ValueTypeF32:
		castVal = uint64(math.Float32frombits(uint32(retVal)))
	case wasm.ValueTypeF64:
		castVal = uint64(math.Float64frombits(retVal))
	default:
		panic("unknown return type")
	}
	return castVal
}
