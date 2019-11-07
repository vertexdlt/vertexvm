package vm

import "github.com/vertexdlt/vertexvm/opcode"

// GasPolicy is the interface for vm cost table
type GasPolicy interface {
	GetCost(op opcode.Opcode) int64
}

// SimpleGasPolicy burn 1 gas for 1 op
type SimpleGasPolicy struct{}

// GetCost return 1 for 1 op
func (p *SimpleGasPolicy) GetCost(op opcode.Opcode) int64 {
	return 1
}
