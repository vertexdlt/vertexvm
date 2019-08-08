package vm

import (
	"fmt"
	"math"

	"github.com/go-interpreter/wagon/wasm"
)

func castReturnValue(retVal int64, retType wasm.ValueType) int64 {
	var castVal int64
	fmt.Println("retType", retType)
	switch retType {
	case wasm.ValueTypeI32:
		castVal = int64(int32(retVal))
	case wasm.ValueTypeI64:
		castVal = retVal
	case wasm.ValueTypeF32:
		castVal = int64(math.Float32frombits(uint32(retVal)))
	case wasm.ValueTypeF64:
		castVal = int64(math.Float64frombits(uint64(retVal)))
	default:
		panic("unknown return type")
	}
	return castVal
}
