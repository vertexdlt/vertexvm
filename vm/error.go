package vm

import "errors"

// ExecError is VM panic-recovered error type
type ExecError struct {
	message string
}

func (e *ExecError) Error() string {
	return e.message
}

// NewExecError creates a new ExecError provided a message string
func NewExecError(message string) *ExecError {
	return &ExecError{message}

}

// ExecError list
var (
	ErrInvalidBreak           = NewExecError("invalid break recover")
	ErrTooManyBrTableTarget   = NewExecError("too many br_table targets")
	ErrIntegerDivisionByZero  = NewExecError("integer divide by zero")
	ErrInvalidIntConversion   = NewExecError("invalid conversion to integer")
	ErrIntegerOverflow        = NewExecError("integer overflow")
	ErrInvalidBreakDepth      = NewExecError("invalid break depth")
	ErrInvalidFunctionBreak   = NewExecError("cannot break out of current function")
	ErrMismatchedFuncSig      = NewExecError("indirect call type mismatch")
	ErrNoMatchingIfBlock      = NewExecError("no matching If for Else block")
	ErrOutOfBoundTableAccess  = NewExecError("out of bounds table access")
	ErrOutOfBoundMemoryAccess = NewExecError("out of bounds memory access")
	ErrUnknownOpcode          = NewExecError("unknown opcode")
	ErrUnknownReturnType      = NewExecError("unknown block return type")
	ErrLebOverflow            = NewExecError("unsigned leb overflow")

	ErrStackOverflow = NewExecError("call stack overflow")
	ErrFrameOverflow = NewExecError("frame stack overflow")
	ErrBlockOverflow = NewExecError("block stack overflow")

	ErrStackUnderflow = NewExecError("call stack underflow")
	ErrFrameUnderflow = NewExecError("no frame to pop")
	ErrBlockUnderflow = NewExecError("cannot find matching block open")

	ErrUnreachable = NewExecError("unreachable")
)

// Non-panic errors
var (
	ErrFuncNotFound      = errors.New("func not found at index")
	ErrInvalidBlockType  = errors.New("invalid block type")
	ErrOutOfGas          = errors.New("out of gas")
	ErrWrongNumberOfArgs = errors.New("wrong number of arguments")
)
