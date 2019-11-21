package vm

import (
	"github.com/go-interpreter/wagon/wasm"
)

func castReturnValue(retVal uint64, retType wasm.ValueType) uint64 {
	var castVal uint64
	switch retType {
	case wasm.ValueTypeI32, wasm.ValueTypeF32:
		castVal = uint64(uint32(retVal))
	case wasm.ValueTypeI64, wasm.ValueTypeF64:
		castVal = retVal
	default:
		panic(ErrUnknownReturnType)
	}
	return castVal
}
