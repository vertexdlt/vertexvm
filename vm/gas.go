package vm

import "github.com/vertexdlt/vertexvm/opcode"

// GasPolicy is the interface for vm cost table
type GasPolicy interface {
	GetCostForOp(op opcode.Opcode) int64
}

// SimpleGasPolicy burn 1 gas for 1 op
type SimpleGasPolicy struct{}

// GetCostForOp return 1 for 1 op
func (p *SimpleGasPolicy) GetCostForOp(op opcode.Opcode) int64 {
	return 1
}
