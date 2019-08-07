package vm

import (
	"bytes"
	"fmt"
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

// MaxFrames is the maxinum active frames supported
const MaxBlocks = 1024

// Frame or call frame holds the relevant execution information of a function
type Frame struct {
	fn             *wasm.Function
	ip             int
	basePointer    int
	baseBlockIndex int
}

type BlockType int

const (
	Norm BlockType = iota + 1
	Loop
	If
	Else
)

type Block struct {
	labelPointer int
	blockType    BlockType
}

// VM virtual machine
type VM struct {
	Module      *wasm.Module
	stack       []int64
	sp          int //point to the next available slot
	frames      []*Frame
	framesIndex int
	globals     []int64
	blocks      []*Block
	blocksIndex int
	breakDepth  int
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
		stack:       make([]int64, StackSize),
		frames:      make([]*Frame, MaxFrames),
		globals:     make([]int64, len(m.GlobalIndexSpace)),
		framesIndex: 0,
		sp:          0,
		blocks:      make([]*Block, MaxBlocks),
		blocksIndex: 0,
		breakDepth:  -1,
	}
	vm.initGlobals()
	return vm, nil
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
	var retVal int64
	var ifExecuted bool
	fmt.Println("instruction", vm.currentFrame().instructions())
	for {
		if vm.currentFrame().hasEnded() {
			hasReturn := len(vm.currentFrame().fn.Sig.ReturnTypes) != 0
			if hasReturn {
				retVal = vm.peek()
				vm.sp = vm.currentFrame().basePointer
				vm.blocksIndex = vm.currentFrame().baseBlockIndex
				vm.push(retVal)
			} else {
				retVal = 0
				vm.sp = vm.currentFrame().basePointer
				vm.blocksIndex = vm.currentFrame().baseBlockIndex
			}
			vm.popFrame()
			if vm.framesIndex == 0 {
				log.Println("End check", vm.sp, vm.framesIndex, vm.blocksIndex)
				return retVal
			}
		}
		vm.currentFrame().ip++
		ins := vm.currentFrame().instructions()
		ip := vm.currentFrame().ip
		op := opcode.Opcode(ins[ip])
		log.Println("instructions", ip, op)
		// log.Println("stack", vm.sp, vm.stack[:10])
		ip++
		if vm.inoperative() {
			switch op {
			case opcode.Block:
			case opcode.Loop:
			case opcode.End:
			case opcode.If:
			case opcode.Else:
			default:
				fmt.Println("skip", op)
				fmt.Println(vm.skipInstructions(op, ip))
				ip += vm.skipInstructions(op, ip)
				vm.currentFrame().ip = ip - 1
				continue
			}
		}
		switch {
		case op == opcode.Unreachable:
			log.Println("unreachable")
		case op == opcode.I32Const:
			val, size := readLEB(ins[ip:], 32, true)
			ip += int(size)
			vm.push(int64(val))
		case opcode.I32Add <= op && op <= opcode.I32Rotr:
			b := int32(vm.pop())
			a := int32(vm.pop())
			var c int32
			switch op {
			case opcode.I32Add:
				fmt.Println("Adding", a, b)
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
				fmt.Println("checking rem", a, b)
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
		case op == opcode.I32Eqz:
			if int32(vm.pop()) == 0 {
				fmt.Println("I32Eqz")
				vm.push(1)
			} else {
				fmt.Println("Not I32Eqz")
				vm.push(0)
			}
		case opcode.I32Eq <= op && op <= opcode.I32GeU:
			b := int32(vm.pop())
			a := int32(vm.pop())
			var c int32
			switch op {
			case opcode.I32Eq:
				c = 0
				if a == b {
					c = 1
				}
			case opcode.I32LtU:
				c = 0
				fmt.Println("Comparing", a, b)
				if uint32(a) < uint32(b) {
					c = 1
				}
			}
			fmt.Println("eq result", c)
			vm.push(int64(c))
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
			// fmt.Println("Setting local", int(arg), frame.basePointer+int(arg), vm.stack[frame.basePointer+int(arg)], vm.stack[:10])
		case op == opcode.GetLocal:
			arg, size := readLEB(ins[ip:], 32, true)
			ip += int(size)
			frame := vm.currentFrame()
			vm.push(vm.stack[frame.basePointer+int(arg)])
			// fmt.Println("Getting local", arg, frame.basePointer+int(arg), vm.stack[frame.basePointer+int(arg)], vm.stack[:10])
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
			second := vm.pop()
			first := vm.pop()
			fmt.Println(cond, first, second)
			if cond == 0 {
				vm.push(second)
			} else {
				vm.push(first)
			}
		case op == opcode.Block:
			block := NewBlock(-1, Norm)
			vm.pushBlock(block)
			if vm.inoperative() {
				vm.breakDepth++
			}
		case op == opcode.End:
			vm.popBlock()
			vm.breakDepth--
		case op == opcode.Br:
			arg, size := readLEB(ins[ip:], 32, true)
			vm.currentFrame().ip += int(size)
			vm.blockJump(int(arg))
			continue
		case op == opcode.BrIf:
			arg, size := readLEB(ins[ip:], 32, true)
			vm.currentFrame().ip += int(size)
			cond := vm.pop()
			fmt.Println("Jump cond", cond, arg)
			if cond != 0 {
				vm.blockJump(int(arg))
			}
			continue
		case op == opcode.Loop:
			block := NewBlock(ip, Loop)
			vm.pushBlock(block)
			if vm.inoperative() {
				vm.breakDepth++
			}
		case op == opcode.If:
			block := NewBlock(ip, If)
			vm.pushBlock(block)
			cond := vm.pop()
			if cond != 0 {
				ifExecuted = true
			} else {
				ifExecuted = false
				fmt.Println("not executing if", cond)
				vm.blockJump(0)
			}
		case op == opcode.Else:
			fmt.Println("entering else")
			vm.popBlock()
			block := NewBlock(ip, Else)
			vm.pushBlock(block)
			if ifExecuted {
				fmt.Println("skipping else")
				vm.blockJump(0)
			} else {
				fmt.Println("executing else", vm.breakDepth)
				vm.breakDepth--
			}
		default:
			log.Println("unknown opcode", op)
		}
		fmt.Println("instruction", op, "stack", vm.stack[:vm.sp], vm.sp)
		vm.currentFrame().ip = ip - 1
	}
}

func (vm *VM) skipInstructions(op opcode.Opcode, ip int) int {
	switch {
	case op == opcode.Br:
		fallthrough
	case op == opcode.BrIf:
		fallthrough
	case op == opcode.Call:
		fallthrough
	case opcode.GetLocal <= op && op <= opcode.SetGlobal:
		fallthrough
	case op == opcode.I32Const:
		_, size := readLEB(vm.currentFrame().instructions()[ip:], 32, false)
		return int(size)
	case op == opcode.I64Const:
		_, size := readLEB(vm.currentFrame().instructions()[ip:], 64, false)
		return int(size)
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

func (vm *VM) inoperative() bool {
	return vm.breakDepth > -1
}

func (vm *VM) blockJump(breakDepth int) {
	if vm.blocksIndex-breakDepth < vm.currentFrame().baseBlockIndex {
		panic("cannot break out of current function")
	}
	fmt.Println(vm.blocks[:vm.blocksIndex], vm.blocksIndex)
	jumpBlock := vm.blocks[vm.blocksIndex-1-breakDepth]
	if jumpBlock.blockType == Loop {
		fmt.Println("jumping to ", jumpBlock.labelPointer)
		vm.currentFrame().ip = jumpBlock.labelPointer
	} else {
		vm.breakDepth = breakDepth
	}
}

func (vm *VM) setupFrame(fidx int) {
	fn := vm.Module.GetFunction(fidx)
	frame := NewFrame(fn, vm.sp-len(fn.Sig.ParamTypes), vm.blocksIndex)
	vm.pushFrame(frame)
	// leave some space for locals
	numVars := len(fn.Sig.ParamTypes)
	for _, entry := range fn.Body.Locals {
		numVars += int(entry.Count)
	}
	vm.sp = frame.basePointer + numVars
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

func (vm *VM) pushBlock(block *Block) {
	if vm.blocksIndex == MaxBlocks {
		panic("Blocks overflow")
	}
	fmt.Println("Pushing block", block, vm.blocksIndex)
	if vm.blocksIndex == 3 {
		panic("what????")
	}
	vm.blocks[vm.blocksIndex] = block
	vm.blocksIndex++
}

func (vm *VM) popBlock() *Block {
	vm.blocksIndex--
	if vm.blocksIndex < vm.currentFrame().baseBlockIndex {
		panic("cannot find matching block openning")
	}
	return vm.blocks[vm.blocksIndex]
}

// GetFunctionIndex look up a function export index by its name
func (vm *VM) GetFunctionIndex(name string) (int64, bool) {
	if entry, ok := vm.Module.Export.Entries[name]; ok {
		return int64(entry.Index), ok
	}
	return -1, false
}

// NewFrame initialize a call frame for a given function fn
func NewFrame(fn *wasm.Function, basePointer int, baseBlockIndex int) *Frame {
	f := &Frame{
		fn:             fn,
		ip:             -1,
		basePointer:    basePointer,
		baseBlockIndex: baseBlockIndex,
	}
	return f
}

func NewBlock(labelPointer int, blockType BlockType) *Block {
	b := &Block{
		labelPointer: labelPointer,
		blockType:    blockType,
	}
	return b
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
			vm.globals[i] = int64(v)
		case int64:
			vm.globals[i] = int64(v)
		case float32:
			vm.globals[i] = int64(math.Float32bits(v))
		case float64:
			vm.globals[i] = int64(math.Float64bits(v))
		}
	}

	return nil
}

func (block *Block) String() string {
	return fmt.Sprintf("[type: %d, pointer: %d]", block.blockType, block.labelPointer)
}
