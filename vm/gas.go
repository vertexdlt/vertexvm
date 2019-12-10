package vm

import "github.com/vertexdlt/vertexvm/opcode"

// Gas consist used and limit for vm execution
type Gas struct {
	Used  uint64
	Limit uint64
}

// GasPolicy is the interface for vm cost table
type GasPolicy interface {
	GetCostForOp(op opcode.Opcode) uint64
}

// FreeGasPolicy free cost
type FreeGasPolicy struct{}

// GetCostForOp return free cost
func (p *FreeGasPolicy) GetCostForOp(op opcode.Opcode) uint64 {
	return 0
}

// SimpleGasPolicy cost 1 gas for 1 op
type SimpleGasPolicy struct{}

// GetCostForOp return 1 for 1 op
func (p *SimpleGasPolicy) GetCostForOp(op opcode.Opcode) uint64 {
	return 1
}
