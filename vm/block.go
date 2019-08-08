package vm

import "github.com/go-interpreter/wagon/wasm"

// BlockType type of a wasm block
type BlockType int

const (
	typeBlock BlockType = iota + 1
	typeLoop
	typeIf
	typeElse
)

// Block holds information related to a WASM block structure
type Block struct {
	labelPointer int //only for Loop Block
	blockType    BlockType
	executed     bool //only for If Block
	returnType   wasm.ValueType
}

// NewBlock initialize a block
func NewBlock(labelPointer int, blockType BlockType, returnType wasm.ValueType) *Block {
	b := &Block{
		labelPointer: labelPointer,
		blockType:    blockType,
		returnType:   returnType,
	}
	return b
}
