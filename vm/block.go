package vm

import (
	"github.com/vertexdlt/vertexvm/opcode"
	"github.com/vertexdlt/vertexvm/wasm"
)

// BlockType type of a wasm block
type BlockType int

const (
	typeBlock BlockType = iota + 1
	typeLoop
	typeIf
)

// Block holds information related to a WASM block structure
type Block struct {
	labelPointer int //only for Loop Block
	blockType    BlockType
	executeElse  bool //only for If Block
	returnType   wasm.ValueType
	basePointer  int
}

// NewBlock initialize a block
func NewBlock(labelPointer int, blockType BlockType, returnType wasm.ValueType, basePointer int) *Block {
	b := &Block{
		labelPointer: labelPointer,
		blockType:    blockType,
		returnType:   returnType,
		basePointer:  basePointer,
		executeElse:  false,
	}
	return b
}

func getBlockType(op opcode.Opcode) BlockType {
	switch op {
	case opcode.Block:
		return typeBlock
	case opcode.Loop:
		return typeLoop
	case opcode.If:
		return typeIf
	default:
		panic("invalid opcode")
	}
}
