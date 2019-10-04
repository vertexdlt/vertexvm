package vm

import "github.com/vertexdlt/vertexvm/opcode"

// Gas is the struct store current state of vm execution
type Gas struct {
	limit uint64
	used  uint64
}

// GetGasCost calculates op cost
func GetGasCost(op opcode.Opcode) uint64 {
	if op != opcode.End {
		return 1
	}
	return 0
}
