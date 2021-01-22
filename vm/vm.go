package vm

import (
	"encoding/binary"
	"io"
	"log"
	"math"
	"math/bits"

	"github.com/vertexdlt/vertexvm/number"
	"github.com/vertexdlt/vertexvm/opcode"
	"github.com/vertexdlt/vertexvm/wasm"
)

// StackSize is the VM stack depth
const StackSize = 64 * 1024

// MaxFrames is the maximum active frames supported
const MaxFrames = 1024

// MaxBlocks is the maximum of nested blocks supported
const MaxBlocks = 1024

// MaxBrTableSize is the maximum number of br_table targets
const MaxBrTableSize = 64 * 1024

const f32SignMask = 1 << 31

const f64SignMask = 1 << 63

const wasmPageSize = 64 * 1024

const maxSize = math.MaxUint32

const f32CanonicalNaNBits = uint64(0x7fc00000)
const f64CanonicalNaNBits = uint64(0x7ff8000000000000)

// HostFunction defines imported functions defined in host
type HostFunction func(vm *VM, args ...uint64) (uint64, error)

// ImportResolver looks up the host imports
type ImportResolver interface {
	GetFunction(module, name string) HostFunction
}

// FunctionImport stores information about host function and the host function itself
type FunctionImport struct {
	module    string
	name      string
	signature *wasm.FuncType
	function  *HostFunction //nolint:structcheck,unused
}

// VM virtual machine
type VM struct {
	Module          *wasm.Module
	stack           []uint64
	sp              int //point to the next available slot
	frames          []*Frame
	framesIndex     int
	globals         []uint64
	blocks          []*Block
	blocksIndex     int
	breakDepth      int
	memory          []byte
	functionImports []FunctionImport
	importResolver  ImportResolver
	gasPolicy       GasPolicy
	gas             *Gas
}

// NewVM initializes a new VM
func NewVM(code []byte, gasPolicy GasPolicy, gas *Gas, importResolver ImportResolver) (*VM, error) {
	m, err := wasm.ReadModule(code)
	if err != nil {
		return nil, err
	}

	if gas.Used > gas.Limit {
		return nil, ErrOutOfGas
	}

	vm := &VM{
		Module:         m,
		stack:          make([]uint64, StackSize),
		frames:         make([]*Frame, MaxFrames),
		globals:        make([]uint64, len(m.GlobalIndexSpace)),
		framesIndex:    0,
		sp:             0,
		blocks:         make([]*Block, MaxBlocks),
		blocksIndex:    0,
		breakDepth:     -1,
		memory:         make([]byte, wasmPageSize),
		importResolver: importResolver,
		gasPolicy:      gasPolicy,
		gas:            gas,
	}
	if m.MemSec != nil && len(m.MemSec.Mems) != 0 {
		n := int(m.MemSec.Mems[0].Limits.Min)
		vm.memory = make([]byte, n*wasmPageSize)
		if len(vm.memory) < len(m.LinearMemoryIndexSpace[0]) {
			return nil, ErrOutOfBoundMemoryAccess
		}
		copy(vm.memory, m.LinearMemoryIndexSpace[0])
		if err := vm.BurnGas(vm.gasPolicy.GetCostForMalloc(n)); err != nil {
			return nil, err
		}
	}

	functionImports := make([]FunctionImport, 0)
	if m.ImportSec != nil {
		for _, entry := range m.ImportSec.Imports {
			switch entry.ImportDesc.Kind {
			case wasm.ExternalFunction:
				typeIndex := entry.ImportDesc.TypeIdx
				functionImports = append(functionImports, FunctionImport{
					module:    entry.ModuleName,
					name:      entry.FieldName,
					signature: &m.TypeSec.FuncTypes[typeIndex],
				})
			default:
				log.Printf("Import type %v not supported\n", entry.ImportDesc.Kind)
			}
		}
	}
	vm.functionImports = functionImports
	if err := vm.initGlobals(); err != nil {
		return nil, err
	}
	if m.StartSec != nil { // called after module loading
		_, err := vm.Invoke(uint64(m.StartSec.FuncIdx)) // start does not take args or return
		if err != nil {
			return nil, err
		}
	}
	return vm, nil
}

// Invoke triggers a WASM function
func (vm *VM) Invoke(fidx uint64, args ...uint64) (ret uint64, err error) {
	defer func() {
		if r := recover(); r != nil {
			switch r.(type) {
			case *ExecError:
				ret, err = 0, r.(error)
			default:
				panic(r)
			}
		}
	}()
	if err := vm.validateFuncArgs(int(fidx), args); err != nil {
		return 0, err
	}

	for _, arg := range args {
		vm.push(arg)
	}
	if err := vm.CallFunction(int(fidx)); err != nil {
		return 0, err
	}
	return vm.interpret()
}

// GetFunctionIndex look up a function export index by its name
func (vm *VM) GetFunctionIndex(name string) (uint64, bool) {
	if vm.Module.ExportSec != nil {
		if entry, ok := vm.Module.ExportSec.ExportMap[name]; ok {
			return uint64(entry.Desc.Idx), ok
		}
	}
	return 0, false
}

// BurnGas for burning gas internal vm and external call
func (vm *VM) BurnGas(cost uint64) error {
	if cost > 0 {
		// log.Printf("Gas limit: %d, used: %d", vm.gasLimit, vm.gasUsed)
		remainingGas := vm.gas.Limit - vm.gas.Used
		if remainingGas < cost {
			return ErrOutOfGas
		}
		vm.gas.Used = vm.gas.Used + cost
	}
	return nil
}

func (vm *VM) burnGasForOp(op opcode.Opcode) error {
	return vm.BurnGas(vm.gasPolicy.GetCostForOp(op))
}

func (vm *VM) interpret() (uint64, error) {
	for {
		for {
			if vm.framesIndex == 0 {
				if vm.sp > 0 {
					return vm.pop(), nil
				}
				return 0, nil
			}
			if vm.currentFrame().hasEnded() {
				vm.popFrame()
			} else {
				break
			}
		}
		frame := vm.currentFrame()
		frame.ip++
		op := opcode.Opcode(frame.instructions()[frame.ip])
		// fmt.Printf("op %d 0x%x\n", op, op)
		if !vm.operative() && vm.skipInstructions(op) {
			continue
		}
		if err := vm.burnGasForOp(op); err != nil {
			return 0, err
		}
		switch {
		case op == opcode.Unreachable:
			log.Println("unreachable")
			panic(ErrUnreachable)
		case op == opcode.Nop:
			continue
		case op == opcode.Block:
			returnType := wasm.ValueType(frame.readLEB(32, false))
			block := NewBlock(frame.ip, typeBlock, returnType, vm.sp)
			vm.pushBlock(block)
		case op == opcode.Loop:
			returnType := wasm.ValueType(frame.readLEB(32, false))
			block := NewBlock(frame.ip, typeLoop, returnType, vm.sp)
			vm.pushBlock(block)
		case op == opcode.If:
			returnType := wasm.ValueType(frame.readLEB(32, true))
			block := NewBlock(frame.ip, typeIf, returnType, vm.sp)
			vm.pushBlock(block)
			cond := vm.pop()
			block.executeElse = (cond == 0)
			if block.executeElse {
				vm.blockJump(0)
			}
		case op == opcode.Else:
			block := vm.blocks[vm.blocksIndex-1]
			if block.blockType != typeIf {
				panic(ErrNoMatchingIfBlock)
			}
			if block.executeElse { // infers vm.operative() == true enterring if
				// if jump 0 so needs to reset in order to resume execution
				vm.breakDepth--
				if vm.breakDepth < -1 {
					panic(ErrInvalidBreak)
				}
			} else {
				if vm.operative() {
					vm.blockJump(0)
				}
			}
		case op == opcode.End:
			block := vm.popBlock()
			if block.basePointer < vm.sp { // block has return value
				if block.returnType != wasm.ValueType(wasm.BlockTypeEmpty) {
					retVal := castReturnValue(vm.pop(), block.returnType)
					vm.push(retVal)
				}
				ret := vm.pop()
				vm.sp = block.basePointer
				vm.push(ret)
			}
			if !vm.operative() {
				vm.breakDepth--
				if vm.breakDepth < -1 {
					panic(ErrInvalidBreak)
				}
			}
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
				panic(ErrTooManyBrTableTarget)
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
			fidx := int(frame.readLEB(32, false))
			if err := vm.CallFunction(fidx); err != nil {
				return 0, err
			}
		case op == opcode.CallIndirect:
			sigIndex := frame.readLEB(32, false)
			expectedFuncSig := wasm.FuncType(vm.Module.TypeSec.FuncTypes[sigIndex])

			frame.readLEB(1, false) // reserve as per https://github.com/WebAssembly/design/blob/master/BinaryEncoding.md#call-operators-described-here
			eidx := vm.pop()
			if int(eidx) >= len(vm.Module.TableIndexSpace[0]) {
				panic(ErrOutOfBoundTableAccess)
			}
			fidx := int(vm.Module.TableIndexSpace[0][eidx])
			if err := vm.CallFunction(fidx); err != nil {
				return 0, err
			}
			if fidx >= len(vm.functionImports) {
				vm.assertFuncSig(fidx, &expectedFuncSig)
			}
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
		case opcode.I32Load <= op && op <= opcode.I64Load32U:
			frame.readLEB(32, false) // alignment
			offset := int(frame.readLEB(32, false))
			address := int(vm.pop())
			address += offset
			vm.assertInbound(address, op.MemAccessSize())
			curMem := vm.memory[address:]
			switch op {
			case opcode.I32Load, opcode.F32Load:
				v := binary.LittleEndian.Uint32(curMem)
				vm.push(uint64(v))
			case opcode.I64Load, opcode.F64Load:
				v := binary.LittleEndian.Uint64(curMem)
				vm.push(v)
			case opcode.I32Load8S, opcode.I64Load8S:
				vm.push(uint64(int8(vm.memory[address])))
			case opcode.I32Load8U, opcode.I64Load8U:
				vm.push(uint64(vm.memory[address]))
			case opcode.I32Load16S, opcode.I64Load16S:
				v := binary.LittleEndian.Uint16(curMem)
				vm.push(uint64(int16(v)))
			case opcode.I32Load16U, opcode.I64Load16U:
				v := binary.LittleEndian.Uint16(curMem)
				vm.push(uint64(v))
			case opcode.I64Load32S:
				v := binary.LittleEndian.Uint32(curMem)
				vm.push(uint64(int32(v)))
			case opcode.I64Load32U:
				v := binary.LittleEndian.Uint32(curMem)
				vm.push(uint64(v))
			}
		case opcode.I32Store <= op && op <= opcode.I64Store32:
			frame.readLEB(32, false) // alignment
			offset := int(frame.readLEB(32, false))
			v := vm.pop()
			address := int(vm.pop())
			address += offset
			vm.assertInbound(address, op.MemAccessSize())
			curMem := vm.memory[address:]
			switch op {
			case opcode.I32Store, opcode.F32Store:
				binary.LittleEndian.PutUint32(curMem, uint32(v))
			case opcode.I64Store, opcode.F64Store:
				binary.LittleEndian.PutUint64(curMem, v)
			case opcode.I32Store8, opcode.I64Store8:
				vm.memory[address] = byte(v)
			case opcode.I32Store16, opcode.I64Store16:
				binary.LittleEndian.PutUint16(curMem, uint16(v))
			case opcode.I64Store32:
				binary.LittleEndian.PutUint32(curMem, uint32(v))
			}
		case op == opcode.MemorySize:
			frame.readLEB(1, false) // reserve as per https://github.com/WebAssembly/design/blob/master/BinaryEncoding.md#memory-related-operators-described-here
			pages := len(vm.memory) / wasmPageSize
			vm.push(uint64(pages))
		case op == opcode.MemoryGrow:
			frame.readLEB(1, false) // reserve as per https://github.com/WebAssembly/design/blob/master/BinaryEncoding.md#memory-related-operators-described-here
			pages := len(vm.memory) / wasmPageSize
			n := int(vm.pop())
			limit := vm.Module.MemSec.Mems[0].Limits
			maxPages := maxSize / wasmPageSize
			if limit.Flag == 1 && maxPages > int(limit.Max) {
				maxPages = int(limit.Max)
			}
			if pages+n >= pages && pages+n <= maxPages {
				vm.memory = append(vm.memory, make([]byte, n*wasmPageSize)...)
				if err := vm.BurnGas(vm.gasPolicy.GetCostForMalloc(n)); err != nil {
					return 0, err
				}
			} else {
				pages = -1
			}
			vm.push(uint64(uint32(pages)))
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
					panic(ErrIntegerDivisionByZero)
				}
				if a == math.MaxInt32+1 && b == math.MaxInt32 {
					panic(ErrIntegerOverflow)
				}
				c = uint32(int32(a) / int32(b))
			case opcode.I32DivU:
				if b == 0 {
					panic(ErrIntegerDivisionByZero)
				}
				c = a / b
			case opcode.I32RemS:
				if b == 0 {
					panic(ErrIntegerDivisionByZero)
				}
				c = uint32(int32(a) % int32(b))
			case opcode.I32RemU:
				if b == 0 {
					panic(ErrIntegerDivisionByZero)
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
					panic(ErrIntegerDivisionByZero)
				}
				if a == math.MaxInt64+1 && b == math.MaxInt64 {
					panic(ErrIntegerOverflow)
				}
				c = uint64(int64(a) / int64(b))
			case opcode.I64DivU:
				if b == 0 {
					panic(ErrIntegerDivisionByZero)
				}
				c = a / b
			case opcode.I64RemS:
				if b == 0 {
					panic(ErrIntegerDivisionByZero)
				}
				c = uint64(int64(a) % int64(b))
			case opcode.I64RemU:
				if b == 0 {
					panic(ErrIntegerDivisionByZero)
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

		case opcode.F32Add <= op && op <= opcode.F32Max:
			bBits := uint32(vm.pop())
			b := math.Float32frombits(bBits)
			aBits := uint32(vm.pop())
			a := math.Float32frombits(aBits)
			var c float32
			switch op {
			case opcode.F32Add:
				c = a + b
			case opcode.F32Sub:
				c = a - b
			case opcode.F32Mul:
				c = a * b
			case opcode.F32Div:
				c = a / b
			case opcode.F32Min:
				c = float32(math.Min(float64(a), float64(b)))
			case opcode.F32Max:
				c = float32(math.Max(float64(a), float64(b)))
			}
			vm.pushFloat32(c)

		// copysign, abs, neg use bitwise to ensure arch independent
		case op == opcode.F32Copysign:
			bBits := uint32(vm.pop())
			aBits := uint32(vm.pop())
			cBits := aBits&^f32SignMask | bBits&f32SignMask
			vm.push(uint64(cBits))

		case op == opcode.F32Neg:
			vm.push(uint64(uint32(vm.pop()) ^ f32SignMask))

		case op == opcode.F32Abs:
			vm.push(uint64(uint32(vm.pop()) &^ f32SignMask))

		case opcode.F32Ceil <= op && op <= opcode.F32Sqrt:
			f := float64(math.Float32frombits(uint32(vm.pop())))
			var r float64
			switch op {
			case opcode.F32Ceil:
				r = math.Ceil(f)
			case opcode.F32Floor:
				r = math.Floor(f)
			case opcode.F32Trunc:
				r = math.Trunc(f)
			case opcode.F32Nearest:
				r = math.RoundToEven(f)
			case opcode.F32Sqrt:
				r = math.Sqrt(f)
			}
			vm.pushFloat32(float32(r))

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

		case opcode.F64Add <= op && op <= opcode.F64Max:
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
			}
			vm.pushFloat64(c)

		// copysign, abs, neg use bitwise to ensure arch independent
		case op == opcode.F64Copysign:
			bBits := vm.pop()
			aBits := vm.pop()
			cBits := aBits&^f64SignMask | bBits&f64SignMask
			vm.push(cBits)

		case op == opcode.F64Neg:
			vm.push(vm.pop() ^ f64SignMask)

		case op == opcode.F64Abs:
			vm.push(vm.pop() &^ f64SignMask)

		case opcode.F64Ceil <= op && op <= opcode.F64Sqrt:
			f := math.Float64frombits(vm.pop())
			var r float64
			switch op {
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
			vm.pushFloat64(r)

		// Conversion
		case op == opcode.I32WrapI64:
			vm.push(uint64(uint32(vm.pop())))
		case opcode.I32TruncSF32 <= op && op <= opcode.I32TruncUF64:
			var r uint64
			var trapCode number.TrapCode
			switch op {
			case opcode.I32TruncSF32:
				r, trapCode = number.FloatTruncate(number.F32, number.I32, vm.pop())
			case opcode.I32TruncUF32:
				r, trapCode = number.FloatTruncate(number.F32, number.U32, vm.pop())
			case opcode.I32TruncSF64:
				r, trapCode = number.FloatTruncate(number.F64, number.I32, vm.pop())
			case opcode.I32TruncUF64:
				r, trapCode = number.FloatTruncate(number.F64, number.U32, vm.pop())
			}
			if trapCode == number.NanTrap {
				panic(ErrInvalidIntConversion)
			} else if trapCode == number.ConvertTrap {
				panic(ErrIntegerOverflow)
			}
			vm.push(r)
		case op == opcode.I64ExtendSI32:
			vm.push(uint64(int64(int32(uint32(vm.pop())))))
		case op == opcode.I64ExtendUI32:
			vm.push(uint64(uint32(vm.pop())))
		case opcode.I64TruncSF32 <= op && op <= opcode.I64TruncUF64:
			var r uint64
			var trapCode number.TrapCode
			switch op {
			case opcode.I64TruncSF32:
				r, trapCode = number.FloatTruncate(number.F32, number.I64, vm.pop())
			case opcode.I64TruncUF32:
				r, trapCode = number.FloatTruncate(number.F32, number.U64, vm.pop())
			case opcode.I64TruncSF64:
				r, trapCode = number.FloatTruncate(number.F64, number.I64, vm.pop())
			case opcode.I64TruncUF64:
				r, trapCode = number.FloatTruncate(number.F64, number.U64, vm.pop())
			}
			if trapCode == number.NanTrap {
				panic(ErrInvalidIntConversion)
			} else if trapCode == number.ConvertTrap {
				panic(ErrIntegerOverflow)
			}
			vm.push(r)
		case op == opcode.F32ConvertSI32:
			i := int32(uint32(vm.pop()))
			vm.push(uint64(math.Float32bits(float32(i))))
		case op == opcode.F32ConvertUI32:
			i := uint32(vm.pop())
			vm.push(uint64(math.Float32bits(float32(i))))
		case op == opcode.F32ConvertSI64:
			i := int64(vm.pop())
			vm.push(uint64(math.Float32bits(float32(i))))
		case op == opcode.F32ConvertUI64:
			i := uint64(vm.pop())
			vm.push(uint64(math.Float32bits(float32(i))))

		case op == opcode.F64ConvertSI32:
			i := int32(uint32(vm.pop()))
			vm.push(uint64(math.Float64bits(float64(i))))
		case op == opcode.F64ConvertUI32:
			i := uint32(vm.pop())
			vm.push(uint64(math.Float64bits(float64(i))))
		case op == opcode.F64ConvertSI64:
			i := int64(vm.pop())
			vm.push(uint64(math.Float64bits(float64(i))))
		case op == opcode.F64ConvertUI64:
			i := uint64(vm.pop())
			vm.push(uint64(math.Float64bits(float64(i))))

		case op == opcode.F32DemoteF64:
			f := math.Float64frombits(vm.pop())
			vm.pushFloat32(float32(f))

		case op == opcode.F64PromoteF32:
			f := math.Float32frombits(uint32(vm.pop()))
			vm.pushFloat64(float64(f))

		case opcode.I32ReinterpretF32 <= op && op <= opcode.F64ReinterpretI64:
			// Do nothing
		case op == opcode.I32Extend8S || op == opcode.I64Extend8S:
			vm.push(uint64(int8(vm.pop())))
		case op == opcode.I32Extend16S || op == opcode.I64Extend16S:
			vm.push(uint64(int16(vm.pop())))
		case op == opcode.I64Extend32S:
			vm.push(uint64(int32(vm.pop())))
		case op == opcode.ITruncSatF:
			subop := frame.readLEB(1, false)
			switch subop {
			case 0: //I32TruncSatF32S
				r, _ := number.FloatTruncate(number.F32, number.I32, vm.pop())
				vm.push(r)
			case 1: //I32TruncSatF32U
				r, _ := number.FloatTruncate(number.F32, number.U32, vm.pop())
				vm.push(r)
			case 2: //I32TruncSatF64S
				r, _ := number.FloatTruncate(number.F64, number.I32, vm.pop())
				vm.push(r)
			case 3: //I32TruncSatF64U
				r, _ := number.FloatTruncate(number.F64, number.U32, vm.pop())
				vm.push(r)
			case 4: //I64TruncSatF32S
				r, _ := number.FloatTruncate(number.F32, number.I64, vm.pop())
				vm.push(r)
			case 5: //I64TruncSatF32U
				r, _ := number.FloatTruncate(number.F32, number.U64, vm.pop())
				vm.push(r)
			case 6: //I64TruncSatF64S
				r, _ := number.FloatTruncate(number.F64, number.I64, vm.pop())
				vm.push(r)
			case 7: //I64TruncSatF64U
				r, _ := number.FloatTruncate(number.F64, number.U64, vm.pop())
				vm.push(r)
			}
		default:
			panic(ErrUnknownOpcode)
		}
	}
}

func (vm *VM) skipInstructions(op opcode.Opcode) bool {
	frame := vm.currentFrame()
	switch {
	case op == opcode.End || op == opcode.Else: // control end
		return false
	case op == opcode.Block || op == opcode.Loop || op == opcode.If:
		returnType := wasm.ValueType(frame.readLEB(32, true))
		block := NewBlock(frame.ip, getBlockType(op), returnType, vm.sp)
		vm.pushBlock(block)
		vm.breakDepth++
	case op == opcode.Br || op == opcode.BrIf || op == opcode.Call:
		fallthrough
	case opcode.GetLocal <= op && op <= opcode.SetGlobal:
		fallthrough
	case op == opcode.I32Const:
		frame.readLEB(32, false)
	case op == opcode.I64Const:
		frame.readLEB(64, false)
	case op == opcode.F32Const:
		frame.readUint32()
	case op == opcode.F64Const:
		frame.readUint64()
	case opcode.I32Load <= op && op <= opcode.I64Store32:
		frame.readLEB(32, false)
		frame.readLEB(32, false)
	case op == opcode.MemorySize || op == opcode.MemoryGrow:
		frame.readLEB(1, false)
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
func (vm *VM) operative() bool {
	return vm.breakDepth == -1
}

func (vm *VM) blockJump(breakDepth int) {
	if breakDepth < 0 {
		panic(ErrInvalidBreakDepth)
	}
	if vm.blocksIndex-breakDepth < vm.currentFrame().baseBlockIndex {
		panic(ErrInvalidFunctionBreak)
	} else if vm.blocksIndex-breakDepth == vm.currentFrame().baseBlockIndex {
		vm.breakDepth = breakDepth
		return
	}
	jumpBlock := vm.blocks[vm.blocksIndex-1-breakDepth]
	if jumpBlock.blockType == typeLoop {
		vm.blocksIndex = vm.blocksIndex - breakDepth
		vm.currentFrame().ip = jumpBlock.labelPointer
	} else {
		vm.breakDepth = breakDepth
	}
}

func (vm *VM) setupFrame(fidx int) error {
	fn := vm.GetFunction(fidx)
	if fn == nil {
		return ErrFuncNotFound
	}
	frame := NewFrame(fn, vm.sp-len(fn.Type.ParamTypes), vm.blocksIndex)
	vm.pushFrame(frame)
	numLocals := 0
	for _, entry := range fn.Code.Locals {
		numLocals += int(entry.Count)
	}
	// leave some space for locals
	vm.sp = frame.basePointer + len(fn.Type.ParamTypes) + numLocals
	// uninitialize locals
	for i := vm.sp - 1; i >= vm.sp-numLocals; i-- {
		vm.stack[i] = 0
	}
	// fmt.Println("Instructions", frame.instructions())
	return nil
}

func (vm *VM) currentFrame() *Frame {
	return vm.frames[vm.framesIndex-1]
}

func (vm *VM) push(val uint64) {
	if vm.sp == StackSize {
		panic(ErrStackOverflow)
	}
	vm.stack[vm.sp] = val
	vm.sp++
}

func (vm *VM) pushFloat32(val float32) {
	if math.IsNaN(float64(val)) {
		vm.push(f32CanonicalNaNBits)
	} else {
		vm.push(uint64(math.Float32bits(val)))
	}
}

func (vm *VM) pushFloat64(val float64) {
	if math.IsNaN(val) {
		vm.push(f64CanonicalNaNBits)
	} else {
		vm.push(math.Float64bits(val))
	}
}

func (vm *VM) pop() uint64 {
	if vm.sp == 0 {
		panic(ErrStackUnderflow)
	}
	vm.sp--
	return vm.stack[vm.sp]
}

func (vm *VM) peek() uint64 {
	if vm.sp == 0 {
		panic(ErrStackUnderflow)
	}
	return vm.stack[vm.sp-1]
}

func (vm *VM) pushFrame(frame *Frame) {
	if vm.framesIndex == MaxFrames {
		panic(ErrFrameOverflow)
	}
	vm.frames[vm.framesIndex] = frame
	vm.framesIndex++
}

func (vm *VM) popFrame() *Frame {
	if vm.framesIndex == 0 {
		panic(ErrFrameUnderflow)
	}
	hasReturn := len(vm.currentFrame().fn.Type.ReturnTypes) != 0
	if hasReturn {
		retVal := castReturnValue(vm.peek(), vm.currentFrame().fn.Type.ReturnTypes[0])
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
		panic(ErrBlockOverflow)
	}
	vm.blocks[vm.blocksIndex] = block
	vm.blocksIndex++
}

func (vm *VM) popBlock() *Block {
	vm.blocksIndex--
	if vm.blocksIndex < vm.currentFrame().baseBlockIndex {
		panic(ErrBlockUnderflow)
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

func (vm *VM) assertFuncSig(fidx int, expectedSignature *wasm.FuncType) {
	signature := vm.GetFunction(fidx).Type
	if len(signature.ParamTypes) != len(expectedSignature.ParamTypes) ||
		len(signature.ReturnTypes) != len(expectedSignature.ReturnTypes) {
		panic(ErrMismatchedFuncSig)
	}
	for i, paramType := range signature.ParamTypes {
		if paramType != expectedSignature.ParamTypes[i] {
			panic(ErrMismatchedFuncSig)
		}
	}
	for i, returnType := range signature.ReturnTypes {
		if returnType != expectedSignature.ReturnTypes[i] {
			panic(ErrMismatchedFuncSig)
		}
	}
}

func (vm *VM) assertInbound(address, accessSize int) {
	if address > vm.MemSize()-accessSize {
		panic(ErrOutOfBoundMemoryAccess)
	}
}

// GetFunction wraps module get function to take imports into account
func (vm *VM) GetFunction(fidx int) *wasm.Function {
	return vm.Module.GetFunction(fidx - len(vm.functionImports))
}

// CheckFunction returns error when the number of parameter input is not match with the required number of parameter
func (vm *VM) validateFuncArgs(fidx int, args []uint64) error {
	var argSize int
	if fidx < len(vm.functionImports) {
		fi := vm.functionImports[fidx]
		argSize = len(fi.signature.ParamTypes)
	} else {
		fn := vm.GetFunction(fidx)
		if fn == nil {
			return ErrFuncNotFound
		}
		argSize = len(fn.Type.ParamTypes)
	}

	if len(args) != argSize {
		return ErrWrongNumberOfArgs
	}
	return nil
}

// CallFunction Either invoke an imported function or align the new frame for the incoming interpretation
func (vm *VM) CallFunction(fidx int) error {
	if fidx < len(vm.functionImports) {
		fi := vm.functionImports[fidx]
		hf := vm.importResolver.GetFunction(fi.module, fi.name)
		argSize := len(fi.signature.ParamTypes)
		args := make([]uint64, argSize)
		for i := argSize - 1; i >= 0; i-- {
			args[i] = vm.pop()
		}
		ret, err := hf(vm, args...)
		vm.push(ret)
		return err
	} else {
		return vm.setupFrame(fidx)
	}
}

// MemSize gets the current vm memory size
func (vm *VM) MemSize() int {
	return len(vm.memory)
}

// MemWrite write a byte buffer to vm memory at a specific offset
func (vm *VM) MemWrite(b []byte, offset int) (int, error) {
	var err error
	if offset+len(b) > vm.MemSize() {
		b = b[:vm.MemSize()-offset]
		err = io.ErrShortWrite
	}
	copy(vm.memory[offset:], b)
	return len(b), err
}

// MemRead copy a vm memory segment to a given placeholder
func (vm *VM) MemRead(b []byte, offset int) (int, error) {
	var err error
	if offset+len(b) > vm.MemSize() {
		b = b[:vm.MemSize()-offset]
		err = io.ErrShortBuffer
	}
	copy(b, vm.memory[offset:offset+len(b)])
	return len(b), err
}

// GetGasUsed exposes the amount of gas burnt for execution
func (vm *VM) GetGasUsed() uint64 {
	return vm.gas.Used
}
