package vm

import (
	"bytes"
	"log"
	"math"
	"math/bits"

	"github.com/go-interpreter/wagon/wasm"
	"github.com/vertexdlt/vertexvm/opcode"
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
	stack       []uint64
	sp          int //point to the next available slot
	frames      []*Frame
	framesIndex int
	globals     []uint64
}

// NewVM initializes a new VM
func NewVM(code []byte) (_retVM *VM, retErr error) {
	reader := bytes.NewReader(code)
	m, err := wasm.ReadModule(reader, nil)
	if err != nil {
		return nil, err
	}
	log.Println(m.FunctionIndexSpace)

	vm := &VM{
		Module:      m,
		stack:       make([]uint64, StackSize),
		frames:      make([]*Frame, MaxFrames),
		framesIndex: 0,
		sp:          0,
	}
	// vm.initGlobals()
	return vm, nil
}

// Invoke triggers a WASM function
func (vm *VM) Invoke(fidx uint64, args ...uint64) uint64 {
	for _, arg := range args {
		vm.push(arg)
	}

	vm.setupFrame(int(fidx))
	ret := vm.interpret()

	return uint64(ret)
}

func (vm *VM) interpret() uint64 {
	var retVal uint64
	for {
		if vm.currentFrame().hasEnded() {
			hasReturn := len(vm.currentFrame().fn.Sig.ReturnTypes) != 0
			if hasReturn {
				retVal = vm.peek()
				vm.sp = vm.currentFrame().basePointer
				vm.push(retVal)
			} else {
				retVal = 0
				vm.sp = vm.currentFrame().basePointer
			}
			vm.popFrame()
			if vm.framesIndex == 0 {
				return retVal
			}
		}
		vm.currentFrame().ip++
		ins := vm.currentFrame().instructions()
		ip := vm.currentFrame().ip
		log.Println("instructions", ins, ip)
		op := opcode.Opcode(ins[ip])
		ip++
		switch {
		case op == opcode.Unreachable:
			log.Println("unreachable")

			// I32 Ops
		case op == opcode.I32Const:
			val, size := readLEB(ins[ip:], 32, false)
			ip += int(size)
			vm.push(uint64(val))
		case op == opcode.I32Eqz:
			if uint32(vm.pop()) == 0 {
				vm.push(1)
			} else {
				vm.push(0)
			}
		case op == opcode.I32Clz:
			vm.push(uint64(bits.LeadingZeros32(uint32(vm.pop()))))
		case op == opcode.I32Ctz:
			vm.push(uint64(bits.TrailingZeros32(uint32(vm.pop()))))
		case op == opcode.I32Popcnt:
			vm.push(uint64(bits.OnesCount32(uint32(vm.pop()))))
		case (opcode.I32Eq <= op && op <= opcode.I32GeU) || (opcode.I32Add <= op && op <= opcode.I32Rotr):
			b := uint32(vm.pop())
			a := uint32(vm.pop())
			var c uint32
			switch op {
			case opcode.I32Eq:
				if a == b {
					c = 1
				} else {
					c = 0
				}
			case opcode.I32Ne:
				if a == b {
					c = 0
				} else {
					c = 1
				}
			case opcode.I32LtS:
				if int32(a) < int32(b) {
					c = 1
				} else {
					c = 0
				}
			case opcode.I32LtU:
				if a < b {
					c = 1
				} else {
					c = 0
				}
			case opcode.I32GtS:
				if int32(a) > int32(b) {
					c = 1
				} else {
					c = 0
				}
			case opcode.I32GtU:
				if a > b {
					c = 1
				} else {
					c = 0
				}
			case opcode.I32LeS:
				if int32(a) <= int32(b) {
					c = 1
				} else {
					c = 0
				}
			case opcode.I32LeU:
				if a <= b {
					c = 1
				} else {
					c = 0
				}
			case opcode.I32GeS:
				if int32(a) >= int32(b) {
					c = 1
				} else {
					c = 0
				}
			case opcode.I32GeU:
				if a >= b {
					c = 1
				} else {
					c = 0
				}
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
				if a == math.MaxInt32+1 && b == math.MaxInt32 {
					panic("signed integer overflow")
				}
				c = uint32(int32(a) / int32(b))
			case opcode.I32DivU:
				if b == 0 {
					panic("integer division by zero")
				}
				c = a / b
			case opcode.I32RemS:
				if b == 0 {
					panic("integer division by zero")
				}
				c = uint32(int32(a) % int32(b))
			case opcode.I32RemU:
				if b == 0 {
					panic("integer division by zero")
				}
				c = a % b
			case opcode.I32And:
				c = a & b
			case opcode.I32Or:
				c = a | b
			case opcode.I32Xor:
				c = a ^ b
			case opcode.I32Shl:
				c = a << (b % 32)
			case opcode.I32ShrS:
				c = uint32(int32(a) >> (b % 32))
			case opcode.I32ShrU:
				c = a >> (b % 32)
			case opcode.I32Rotl:
				c = bits.RotateLeft32(a, int(b))
			case opcode.I32Rotr:
				c = bits.RotateLeft32(a, int(-b))
			}
			vm.push(uint64(c))

		// I64 Ops
		case op == opcode.I64Const:
			val, size := readLEB(ins[ip:], 64, false)
			ip += int(size)
			vm.push(uint64(val))
		case op == opcode.I64Eqz:
			if vm.pop() == 0 {
				vm.push(1)
			} else {
				vm.push(0)
			}
		case op == opcode.I64Clz:
			vm.push(uint64(bits.LeadingZeros64(vm.pop())))
		case op == opcode.I64Ctz:
			vm.push(uint64(bits.TrailingZeros64(vm.pop())))
		case op == opcode.I64Popcnt:
			vm.push(uint64(bits.OnesCount64(vm.pop())))
		case (opcode.I64Eq <= op && op <= opcode.I64GeU) || (opcode.I64Add <= op && op <= opcode.I64Rotr):
			b := vm.pop()
			a := vm.pop()
			var c uint64
			switch op {
			case opcode.I64Eq:
				if a == b {
					c = 1
				} else {
					c = 0
				}
			case opcode.I64Ne:
				if a == b {
					c = 0
				} else {
					c = 1
				}
			case opcode.I64LtS:
				if int64(a) < int64(b) {
					c = 1
				} else {
					c = 0
				}
			case opcode.I64LtU:
				if a < b {
					c = 1
				} else {
					c = 0
				}
			case opcode.I64GtS:
				if int64(a) > int64(b) {
					c = 1
				} else {
					c = 0
				}
			case opcode.I64GtU:
				if a > b {
					c = 1
				} else {
					c = 0
				}
			case opcode.I64LeS:
				if int64(a) <= int64(b) {
					c = 1
				} else {
					c = 0
				}
			case opcode.I64LeU:
				if a <= b {
					c = 1
				} else {
					c = 0
				}
			case opcode.I64GeS:
				if int64(a) >= int64(b) {
					c = 1
				} else {
					c = 0
				}
			case opcode.I64GeU:
				if a >= b {
					c = 1
				} else {
					c = 0
				}
			case opcode.I64Add:
				c = a + b
			case opcode.I64Sub:
				c = a - b
			case opcode.I64Mul:
				c = a * b
			case opcode.I64DivS:
				if b == 0 {
					panic("integer division by zero")
				}
				if a == math.MaxInt64+1 && b == math.MaxInt64 {
					panic("signed integer overflow")
				}
				c = uint64(int64(a) / int64(b))
			case opcode.I64DivU:
				if b == 0 {
					panic("integer division by zero")
				}
				c = a / b
			case opcode.I64RemS:
				if b == 0 {
					panic("integer division by zero")
				}
				c = uint64(int64(a) % int64(b))
			case opcode.I64RemU:
				if b == 0 {
					panic("integer division by zero")
				}
				c = a % b
			case opcode.I64And:
				c = a & b
			case opcode.I64Or:
				c = a | b
			case opcode.I64Xor:
				c = a ^ b
			case opcode.I64Shl:
				c = a << (b % 64)
			case opcode.I64ShrS:
				c = uint64(int64(a) >> (b % 64))
			case opcode.I64ShrU:
				c = a >> (b % 64)
			case opcode.I64Rotl:
				c = bits.RotateLeft64(a, int(b))
			case opcode.I64Rotr:
				c = bits.RotateLeft64(a, int(-b))
			}
			vm.push(c)

		case op == opcode.Return:
			return vm.pop()
		case op == opcode.Call:
			fidx, size := readLEB(ins[ip:], 32, true)
			ip += int(size)
			vm.setupFrame(int(fidx))
			continue
		case op == opcode.SetLocal:
			arg, size := readLEB(ins[ip:], 32, true)
			ip += int(size)
			frame := vm.currentFrame()
			vm.stack[frame.basePointer+int(arg)] = vm.pop()
		case op == opcode.GetLocal:
			arg, size := readLEB(ins[ip:], 32, true)
			ip += int(size)
			frame := vm.currentFrame()
			vm.push(vm.stack[frame.basePointer+int(arg)])
		case op == opcode.TeeLocal:
			arg, size := readLEB(ins[ip:], 32, true)
			ip += int(size)
			frame := vm.currentFrame()
			vm.stack[frame.basePointer+int(arg)] = vm.peek()
		case op == opcode.GetGlobal:
			arg, size := readLEB(ins[ip:], 32, true)
			ip += int(size)
			vm.push(vm.globals[arg])
		case op == opcode.SetGlobal:
			arg, size := readLEB(ins[ip:], 32, true)
			ip += int(size)
			vm.globals[arg] = vm.pop()
		case op == opcode.Drop:
			vm.pop()
		case op == opcode.Select:
			cond := vm.pop()
			first := vm.pop()
			second := vm.pop()
			if cond == 0 {
				vm.push(second)
			} else {
				vm.push(first)
			}
		default:
			log.Println("unknown opcode", op)
		}
		vm.currentFrame().ip = ip - 1
	}
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
	vm.pushFrame(frame)
	// leave some space for locals
	vm.sp = frame.basePointer + len(fn.Body.Locals) + len(fn.Sig.ParamTypes)
}

func (vm *VM) currentFrame() *Frame {
	return vm.frames[vm.framesIndex-1]
}

func (vm *VM) push(val uint64) {
	if vm.sp == StackSize {
		panic("Stack overflow")
	}
	vm.stack[vm.sp] = val
	vm.sp++
}

func (vm *VM) pop() uint64 {
	vm.sp--
	return vm.stack[vm.sp]
}

func (vm *VM) peek() uint64 {
	return vm.stack[vm.sp-1]
}

func (vm *VM) pushFrame(frame *Frame) {
	if vm.framesIndex == MaxFrames {
		panic("Frames overflow")
	}
	vm.frames[vm.framesIndex] = frame
	vm.framesIndex++
}

func (vm *VM) popFrame() *Frame {
	vm.framesIndex--
	return vm.frames[vm.framesIndex]
}

// GetFunctionIndex look up a function export index by its name
func (vm *VM) GetFunctionIndex(name string) (uint64, bool) {
	if entry, ok := vm.Module.Export.Entries[name]; ok {
		return uint64(entry.Index), ok
	}
	return 0, false
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

func (vm *VM) initGlobals() error {
	for i, global := range vm.Module.GlobalIndexSpace {
		val, err := vm.Module.ExecInitExpr(global.Init)
		if err != nil {
			return err
		}
		switch v := val.(type) {
		case int32:
			vm.globals[i] = uint64(v)
		case int64:
			vm.globals[i] = uint64(v)
		case float32:
			vm.globals[i] = uint64(math.Float32bits(v))
		case float64:
			vm.globals[i] = uint64(math.Float64bits(v))
		}
	}

	return nil
}
