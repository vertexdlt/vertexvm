package vm

import (
	"bytes"
	"log"
	"math"
	"math/bits"

	"github.com/chewxy/math32"
	"github.com/go-interpreter/wagon/wasm"
	"github.com/vertexdlt/vertexvm/opcode"
)

// StackSize is the VM stack depth
const StackSize = 1024 * 8

// MaxFrames is the maximum active frames supported
const MaxFrames = 1024

// MaxBlocks is the maximum of nested blocks supported
const MaxBlocks = 1024

// MaxBrTableSize is the maximum number of br_table targets
const MaxBrTableSize = 64 * 1024

const f32SignMask = 1 << 31

// VM virtual machine
type VM struct {
	Module      *wasm.Module
	stack       []uint64
	sp          int //point to the next available slot
	frames      []*Frame
	framesIndex int
	globals     []uint64
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

	vm := &VM{
		Module:      m,
		stack:       make([]uint64, StackSize),
		frames:      make([]*Frame, MaxFrames),
		globals:     make([]uint64, len(m.GlobalIndexSpace)),
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
func (vm *VM) Invoke(fidx uint64, args ...uint64) uint64 {
	for _, arg := range args {
		vm.push(arg)
	}

	vm.setupFrame(int(fidx))
	return vm.interpret()
}

// GetFunctionIndex look up a function export index by its name
func (vm *VM) GetFunctionIndex(name string) (uint64, bool) {
	if entry, ok := vm.Module.Export.Entries[name]; ok {
		return uint64(entry.Index), ok
	}
	return 0, false
}

func (vm *VM) interpret() uint64 {
	for {
		for {
			if vm.currentFrame().hasEnded() {
				// fmt.Println("pop frame", vm.framesIndex-1, vm.stack[:10], vm.sp)
				vm.popFrame()
				if vm.framesIndex == 0 {
					if vm.sp > 0 {
						return vm.pop()
					}
					return 0
				}
			} else {
				break
			}
		}
		frame := vm.currentFrame()
		frame.ip++
		op := opcode.Opcode(frame.instructions()[frame.ip])
		// fmt.Printf("op %d 0x%x\n", op, op)
		if vm.inoperative() && vm.skipInstructions(op) {
			continue
		}
		switch {
		case op == opcode.Unreachable:
			log.Println("unreachable")

		case op == opcode.Nop:
			continue
		case op == opcode.Block:
			returnType := wasm.ValueType(frame.readLEB(32, true))
			block := NewBlock(frame.ip, typeBlock, returnType, vm.sp)
			vm.pushBlock(block)
			if vm.inoperative() {
				vm.breakDepth++
			}
		case op == opcode.Loop:
			returnType := wasm.ValueType(frame.readLEB(32, true))
			block := NewBlock(frame.ip, typeLoop, returnType, vm.sp)
			vm.pushBlock(block)
			if vm.inoperative() {
				vm.breakDepth++
			}
		case op == opcode.If:
			returnType := wasm.ValueType(frame.readLEB(32, true))
			block := NewBlock(frame.ip, typeIf, returnType, vm.sp)
			vm.pushBlock(block)
			block.executeElse = false
			if !vm.inoperative() {
				cond := vm.pop()
				block.executeElse = (cond == 0)
				if block.executeElse {
					vm.blockJump(0)
				}
			}
		case op == opcode.Else:
			ifBlock := vm.popBlock()
			if ifBlock.blockType != typeIf {
				log.Fatal("No matching If for Else block")
			}
			block := NewBlock(frame.ip, typeElse, ifBlock.returnType, ifBlock.basePointer)
			vm.pushBlock(block)
			// todo: consider removing Else block
			if ifBlock.executeElse {
				// if jump 0 so needs to reset in order to resume execution
				vm.breakDepth--
			}
			if !vm.inoperative() {
				if !ifBlock.executeElse {
					vm.blockJump(0)
				}
			}
		case op == opcode.End:
			block := vm.popBlock()
			if block.basePointer < vm.sp {
				if block.returnType != wasm.ValueType(wasm.BlockTypeEmpty) {
					retVal := castReturnValue(vm.pop(), block.returnType)
					vm.push(retVal)
				}
				ret := vm.pop()
				vm.sp = block.basePointer
				vm.push(ret)
			}
			vm.breakDepth--
		case op == opcode.Br:
			arg := frame.readLEB(32, false)
			vm.blockJump(int(arg))
			continue
		case op == opcode.BrIf:
			arg := frame.readLEB(32, false)
			cond := vm.pop()
			if cond != 0 {
				vm.blockJump(int(arg))
			}
			continue
		case op == opcode.BrTable:
			targetIndex := int(vm.pop())
			targetCount := int(frame.readLEB(32, false))
			targetDepth := -1
			if targetCount > MaxBrTableSize {
				panic("Too many br_table targets")
			}
			for i := 0; i < targetCount+1; i++ { // +1 for default target
				depth := int(frame.readLEB(32, false))
				if i == targetIndex || i == targetCount {
					if targetDepth == -1 { // uninitialized
						targetDepth = depth
					}
				}
			}
			vm.blockJump(targetDepth)
			continue
		case op == opcode.Return:
			// TODO validate jump
			vm.blockJump(vm.blocksIndex - frame.baseBlockIndex)
		case op == opcode.Call:
			fidx := frame.readLEB(32, false)
			vm.setupFrame(int(fidx))
			continue
		case op == opcode.CallIndirect:
			sigIndex := frame.readLEB(32, false)
			expectedFuncSig := wasm.FunctionSig(vm.Module.Types.Entries[sigIndex])

			frame.readLEB(1, false) // reserve as per https://github.com/WebAssembly/design/blob/master/BinaryEncoding.md#call-operators-described-here
			eidx := vm.pop()
			if int(eidx) >= len(vm.Module.TableIndexSpace[0]) {
				log.Fatal("Out of bound table access")
			}
			fidx := int(vm.Module.TableIndexSpace[0][eidx])
			vm.assertFuncSig(fidx, &expectedFuncSig)
			vm.setupFrame(int(fidx))
			continue
		case op == opcode.Drop:
			vm.pop()
		case op == opcode.Select:
			cond := vm.pop()
			second := vm.pop()
			first := vm.pop()
			if cond == 0 {
				vm.push(second)
			} else {
				vm.push(first)
			}
		case op == opcode.GetLocal:
			arg := frame.readLEB(32, false)
			frame := vm.currentFrame()
			vm.push(vm.stack[frame.basePointer+int(arg)])
		case op == opcode.SetLocal:
			arg := frame.readLEB(32, false)
			frame := vm.currentFrame()
			vm.stack[frame.basePointer+int(arg)] = vm.pop()
		case op == opcode.TeeLocal:
			arg := frame.readLEB(32, false)
			frame := vm.currentFrame()
			vm.stack[frame.basePointer+int(arg)] = vm.peek()
		case op == opcode.GetGlobal:
			arg := frame.readLEB(32, false)
			vm.push(vm.globals[arg])
		case op == opcode.SetGlobal:
			arg := frame.readLEB(32, false)
			vm.globals[arg] = vm.pop()

		// I32 Ops
		case op == opcode.I32Const:
			val := frame.readLEB(32, true)
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
			val := frame.readLEB(64, true)
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

		// F32 Ops
		case op == opcode.F32Const:
			val := frame.readUint32()
			vm.push(uint64(val))
		case opcode.F32Eq <= op && op <= opcode.F32Ge:
			b := math.Float32frombits(uint32(vm.pop()))
			a := math.Float32frombits(uint32(vm.pop()))
			var c uint64
			switch op {
			case opcode.F32Eq:
				if a == b {
					c = 1
				} else {
					c = 0
				}
			case opcode.F32Ne:
				if a == b {
					c = 0
				} else {
					c = 1
				}
			case opcode.F32Lt:
				if a < b {
					c = 1
				} else {
					c = 0
				}
			case opcode.F32Gt:
				if a > b {
					c = 1
				} else {
					c = 0
				}
			case opcode.F32Le:
				if a <= b {
					c = 1
				} else {
					c = 0
				}
			case opcode.F32Ge:
				if a >= b {
					c = 1
				} else {
					c = 0
				}
			}
			vm.push(c)

		case opcode.F32Add <= op && op <= opcode.F32Copysign:
			bBits := uint32(vm.pop())
			b := math.Float32frombits(bBits)
			aBits := uint32(vm.pop())
			a := math.Float32frombits(aBits)
			var cBits uint32
			switch op {
			case opcode.F32Add:
				cBits = math.Float32bits(a + b)
			case opcode.F32Sub:
				cBits = math.Float32bits(a - b)
			case opcode.F32Mul:
				cBits = math.Float32bits(a * b)
			case opcode.F32Div:
				cBits = math.Float32bits(a / b)
			case opcode.F32Min:
				cBits = aBits
				if a > b || (a == b && bBits&f32SignMask != 0) {
					cBits = bBits
				}
			case opcode.F32Max:
				cBits = aBits
				if a < b || (a == b && bBits&f32SignMask == 0) {
					cBits = bBits
				}
			case opcode.F32Copysign:
				cBits = math.Float32bits(a)&^f32SignMask | math.Float32bits(b)&f32SignMask
			}
			vm.push(uint64(cBits))

		case opcode.F32Abs <= op && op <= opcode.F32Sqrt:
			fBits := uint32(vm.pop())
			f := math.Float32frombits(fBits)
			var rBits uint32
			switch op {
			case opcode.F32Abs:
				rBits = fBits &^ f32SignMask
			case opcode.F32Neg:
				rBits = fBits ^ f32SignMask
			case opcode.F32Ceil:
				rBits = math.Float32bits(math32.Ceil(f))
			case opcode.F32Floor:
				rBits = math.Float32bits(math32.Floor(f))
			case opcode.F32Trunc:
				rBits = math.Float32bits(math32.Trunc(f))
			case opcode.F32Nearest:
				t := math32.Trunc(f)
				odd := math32.Remainder(t, 2) != 0
				if d := math32.Abs(f - t); d > 0.5 || (d == 0.5 && odd) {
					t = t + math32.Copysign(1, f)
				}
				rBits = math.Float32bits(t)
			case opcode.F32Sqrt:
				rBits = math.Float32bits(math32.Sqrt(f))
			}

			vm.push(uint64(rBits))

		// F64 Ops
		case op == opcode.F64Const:
			val := frame.readUint64()
			vm.push(uint64(val))
		case opcode.F64Eq <= op && op <= opcode.F64Ge:
			b := math.Float64frombits(vm.pop())
			a := math.Float64frombits(vm.pop())
			var c uint64
			switch op {
			case opcode.F64Eq:
				if a == b {
					c = 1
				} else {
					c = 0
				}
			case opcode.F64Ne:
				if a == b {
					c = 0
				} else {
					c = 1
				}
			case opcode.F64Lt:
				if a < b {
					c = 1
				} else {
					c = 0
				}
			case opcode.F64Gt:
				if a > b {
					c = 1
				} else {
					c = 0
				}
			case opcode.F64Le:
				if a <= b {
					c = 1
				} else {
					c = 0
				}
			case opcode.F64Ge:
				if a >= b {
					c = 1
				} else {
					c = 0
				}
			}
			vm.push(c)

		case opcode.F64Add <= op && op <= opcode.F64Copysign:
			b := math.Float64frombits(vm.pop())
			a := math.Float64frombits(vm.pop())
			var c float64
			switch op {
			case opcode.F64Add:
				c = a + b
			case opcode.F64Sub:
				c = a - b
			case opcode.F64Mul:
				c = a * b
			case opcode.F64Div:
				c = a / b
			case opcode.F64Min:
				c = math.Min(a, b)
			case opcode.F64Max:
				c = math.Max(a, b)
			case opcode.F64Copysign:
				c = math.Copysign(a, b)
			}
			vm.push(math.Float64bits(c))

		case opcode.F64Abs <= op && op <= opcode.F64Sqrt:
			f := math.Float64frombits(vm.pop())
			var r float64
			switch op {
			case opcode.F64Abs:
				r = math.Abs(f)
			case opcode.F64Neg:
				r = -f
			case opcode.F64Ceil:
				r = math.Ceil(f)
			case opcode.F64Floor:
				r = math.Floor(f)
			case opcode.F64Trunc:
				r = math.Trunc(f)
			case opcode.F64Nearest:
				r = math.RoundToEven(f)
			case opcode.F64Sqrt:
				r = math.Sqrt(f)
			}

			vm.push(math.Float64bits(r))

		default:
			log.Printf("unknown opcode 0x%x\n", op)
		}
	}
}

func (vm *VM) skipInstructions(op opcode.Opcode) bool {
	frame := vm.currentFrame()
	switch {
	case op == opcode.Block || op == opcode.Loop || op == opcode.End || op == opcode.If || op == opcode.Else:
		return false
	case op == opcode.Br || op == opcode.BrIf || op == opcode.Call:
		fallthrough
	case opcode.GetLocal <= op && op <= opcode.SetGlobal:
		fallthrough
	case op == opcode.I32Const:
		frame.readLEB(32, false)
	case op == opcode.I64Const:
		frame.readLEB(64, false)
	case op == opcode.CallIndirect:
		frame.readLEB(32, false)
		frame.readLEB(1, false)
	case op == opcode.BrTable:
		targetCount := int(frame.readLEB(32, false))
		for i := 0; i < targetCount+1; i++ {
			frame.readLEB(32, false)
		}
	}
	return true
}

// inoperative vm skips instructions if there is at least 1 level of block to break out of
func (vm *VM) inoperative() bool {
	return vm.breakDepth > -1
}

func (vm *VM) blockJump(breakDepth int) {
	if breakDepth < 0 {
		panic("Invalid break depth")
	}
	if vm.blocksIndex-breakDepth < vm.currentFrame().baseBlockIndex {
		panic("cannot break out of current function")
	} else if vm.blocksIndex-breakDepth == vm.currentFrame().baseBlockIndex {
		vm.breakDepth = breakDepth
		return
	}
	jumpBlock := vm.blocks[vm.blocksIndex-1-breakDepth]
	if jumpBlock.blockType == typeLoop {
		vm.currentFrame().ip = jumpBlock.labelPointer
	} else {
		vm.breakDepth = breakDepth
	}
}

func (vm *VM) setupFrame(fidx int) {
	fn := vm.Module.GetFunction(fidx)
	frame := NewFrame(fn, vm.sp-len(fn.Sig.ParamTypes), vm.blocksIndex)
	vm.pushFrame(frame)
	numLocals := 0
	for _, entry := range fn.Body.Locals {
		numLocals += int(entry.Count)
	}
	// leave some space for locals
	vm.sp = frame.basePointer + len(fn.Sig.ParamTypes) + numLocals
	// uninitialize locals
	for i := vm.sp - 1; i >= vm.sp-numLocals; i-- {
		vm.stack[i] = 0
	}
	// fmt.Println("Instructions", frame.instructions())
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
	hasReturn := len(vm.currentFrame().fn.Sig.ReturnTypes) != 0
	if hasReturn {
		retVal := castReturnValue(vm.peek(), vm.currentFrame().fn.Sig.ReturnTypes[0])
		vm.sp = vm.currentFrame().basePointer
		vm.blocksIndex = vm.currentFrame().baseBlockIndex
		vm.push(retVal)
	} else {
		vm.sp = vm.currentFrame().basePointer
		vm.blocksIndex = vm.currentFrame().baseBlockIndex
	}
	vm.breakDepth = -1 // return reset
	vm.framesIndex--
	return vm.frames[vm.framesIndex]
}

func (vm *VM) pushBlock(block *Block) {
	if vm.blocksIndex == MaxBlocks {
		panic("Blocks overflow")
	}
	vm.blocks[vm.blocksIndex] = block
	vm.blocksIndex++
}

func (vm *VM) popBlock() *Block {
	vm.blocksIndex--
	if vm.blocksIndex < vm.currentFrame().baseBlockIndex {
		panic("cannot find matching block opening")
	}
	return vm.blocks[vm.blocksIndex]
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

func (vm *VM) assertFuncSig(fidx int, expectedSignature *wasm.FunctionSig) {
	signature := vm.Module.GetFunction(fidx).Sig
	if len(signature.ParamTypes) != len(expectedSignature.ParamTypes) ||
		len(signature.ReturnTypes) != len(expectedSignature.ReturnTypes) {
		panic("Mismatch function signature")
	}
	for i, paramType := range signature.ParamTypes {
		if paramType != expectedSignature.ParamTypes[i] {
			panic("Mismatch function signature")
		}
	}
	for i, returnType := range signature.ReturnTypes {
		if returnType != expectedSignature.ReturnTypes[i] {
			panic("Mismatch function signature")
		}
	}
}
