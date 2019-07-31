package vm

import (
	"bytes"
	"log"
	"math"
	"math/bits"

	"github.com/go-interpreter/wagon/wasm"
	"github.com/vertexdlt/vm/opcode"
)

// StackSize is the VM stack depth
const StackSize = 1024 * 8

// MaxFrames is the maxinum active frames supported
const MaxFrames = 1024

// Frame or call frame holds the relevant execution information of a function
type Frame struct {
	fn          *wasm.Function
	ip          int
	basePointer int
}

// VM virtual machine
type VM struct {
	Module      *wasm.Module
	stack       []int64
	sp          int //point to the next available slot
	frames      []*Frame
	framesIndex int
}

// NewVM initializes a new VM
func NewVM(code []byte) (_retVM *VM, retErr error) {
	reader := bytes.NewReader(code)
	m, err := wasm.ReadModule(reader, nil)
	if err != nil {
		return nil, err
	}
	log.Println(m.FunctionIndexSpace)

	return &VM{
		Module:      m,
		stack:       make([]int64, StackSize),
		frames:      make([]*Frame, MaxFrames),
		framesIndex: 0,
		sp:          0,
	}, nil
}

// Invoke triggers a WASM function
func (vm *VM) Invoke(fidx int64, args ...int64) int64 {
	for _, arg := range args {
		vm.push(arg)
	}

	vm.setupFrame(int(fidx))
	ret := vm.interpret()

	return int64(ret)
}

func (vm *VM) interpret() int64 {
	for vm.framesIndex > 1 || !vm.currentFrame().hasEnded() {
		if vm.currentFrame().hasEnded() {
			vm.framesIndex--
		}
		vm.currentFrame().ip++
		ins := vm.currentFrame().instructions()
		i := vm.currentFrame().ip
		log.Println("instructions", ins, i)
		op := opcode.Opcode(ins[i])
		i++
		switch {
		case op == opcode.Unreachable:
			log.Println("unreachable")
		case op == opcode.I32Const:
			val, size := readLEB(ins[i:], 32, true)
			i += int(size)
			// log.Println("i32.const", val, size)
			vm.push(int64(val))
		case opcode.I32Add <= op && op <= opcode.I32Rotr:
			b := int32(vm.pop())
			a := int32(vm.pop())
			var c int32
			switch op {
			case opcode.I32Add:
				c = a + b
			case opcode.I32Sub:
				c = a - b
			case opcode.I32Mul:
				c = a * b
			case opcode.I32DivS:
				if b == 0 {
					panic("integer division by zero")
				}
				if a == math.MinInt32 && b == -1 {
					panic("signed integer overflow")
				}
				c = a / b
			case opcode.I32DivU:
				if b == 0 {
					panic("integer division by zero")
				}
				c = int32(uint32(a) / uint32(b))
			case opcode.I32RemS:
				if b == 0 {
					panic("integer division by zero")
				}
				c = a % b
			case opcode.I32RemU:
				if b == 0 {
					panic("integer division by zero")
				}
				c = int32(uint32(a) % uint32(b))
			case opcode.I32And:
				c = a & b
			case opcode.I32Or:
				c = a | b
			case opcode.I32Xor:
				c = a ^ b
			case opcode.I32Shl:
				c = a << (uint32(b) % 32)
			case opcode.I32ShrS:
				c = a >> uint32(b)
			case opcode.I32ShrU:
				c = int32(uint32(a) >> uint32(b))
			case opcode.I32Rotl:
				c = int32(bits.RotateLeft32(uint32(a), int(b)))
			case opcode.I32Rotr:
				c = int32(bits.RotateLeft32(uint32(a), int(-b)))
			}
			vm.push(int64(c))
		case op == opcode.Return:
			return vm.pop()
		case op == opcode.Call:
			fidx, size := readLEB(ins[i:], 32, true)
			i += int(size)
			vm.setupFrame(int(fidx))
			continue

		case op == opcode.SetLocal:
			arg, size := readLEB(ins[i:], 32, true)
			i += int(size)
			val := vm.pop()
			frame := vm.currentFrame()
			vm.stack[frame.basePointer+int(arg)] = val

		case op == opcode.GetLocal:
			arg, size := readLEB(ins[i:], 32, true)
			i += int(size)
			frame := vm.currentFrame()
			vm.push(vm.stack[frame.basePointer+int(arg)])
			log.Println("Local retrieved", vm.stack[vm.sp-1])
		default:
			log.Println("unknown opcode", op)
		}
		vm.currentFrame().ip = i - 1
	}
	hasReturn := len(vm.currentFrame().fn.Sig.ReturnTypes) != 0
	vm.framesIndex--
	if hasReturn {
		return vm.peek()
	}
	return 0
}

func readLEB(bytes []byte, maxbit uint32, hasSign bool) (int64, uint32) {
	var (
		shift  uint32
		bitcnt uint32
		cur    int64
		result int64
		sign   int64 = -1
	)
	for i := 0; i < len(bytes); i++ {
		cur = int64(bytes[i])
		result |= (cur & 0x7f) << shift
		shift += 7
		sign <<= 7
		bitcnt++
		if cur&0x80 == 0 {
			break
		}
		if bitcnt > (maxbit+7-1)/7 {
			log.Fatal("Unsigned LEB at byte overflow")
		}
	}
	if hasSign && ((sign>>1)&result) != 0 {
		result |= sign
	}
	return result, bitcnt
}

func (vm *VM) setupFrame(fidx int) {
	fn := vm.Module.GetFunction(fidx)
	frame := NewFrame(fn, vm.sp-len(fn.Sig.ParamTypes))
	vm.frames[vm.framesIndex] = frame
	vm.framesIndex++
	// leave some space for locals
	vm.sp = frame.basePointer + len(fn.Body.Locals) + len(fn.Sig.ParamTypes)
}

func (vm *VM) currentFrame() *Frame {
	return vm.frames[vm.framesIndex-1]
}

func (vm *VM) push(val int64) {
	if vm.sp == StackSize {
		panic("Stack overflow")
	}
	vm.stack[vm.sp] = val
	vm.sp++
}

func (vm *VM) pop() int64 {
	vm.sp--
	return vm.stack[vm.sp]
}

func (vm *VM) peek() int64 {
	return vm.stack[vm.sp-1]
}

// GetFunctionIndex look up a function export index by its name
func (vm *VM) GetFunctionIndex(name string) (int64, bool) {
	if entry, ok := vm.Module.Export.Entries[name]; ok {
		return int64(entry.Index), ok
	}
	return -1, false
}

// NewFrame initialize a call frame for a given function fn
func NewFrame(fn *wasm.Function, basePointer int) *Frame {
	f := &Frame{
		fn:          fn,
		ip:          -1,
		basePointer: basePointer,
	}
	return f
}

func (frame *Frame) instructions() []byte {
	return frame.fn.Body.Code
}

func (frame *Frame) hasEnded() bool {
	return frame.ip == len(frame.instructions())-1
}
