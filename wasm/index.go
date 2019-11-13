package wasm

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/vertexdlt/vertexvm/leb128"
	"github.com/vertexdlt/vertexvm/util"
)

const (
	i32Const  byte = 0x41
	i64Const  byte = 0x42
	f32Const  byte = 0x43
	f64Const  byte = 0x44
	getGlobal byte = 0x23
	end       byte = 0x0b
)

type Function struct {
	Type FuncType
	Code Code
	Name string
}

// Module represent Wasm Module
// https://webassembly.github.io/spec/core/binary/modules.html#binary-module
type Module struct {
	Version uint32

	TypeSec    *TypeSec
	ImportSec  *ImportSec
	FuncSec    *FuncSec
	TableSec   *TableSec
	MemSec     *MemSec
	GlobalSec  *GlobalSec
	ExportSec  *ExportSec
	StartSec   *StartSec
	ElementSec *ElementSec
	CodeSec    *CodeSec
	DataSec    *DataSec

	FunctionIndexSpace []Function
	GlobalIndexSpace   []Global

	TableIndexSpace        [][]uint32
	LinearMemoryIndexSpace [][]byte
}

func (m *Module) ExecInitExpr(expr []byte) (interface{}, error) {
	var stack []uint64
	var lastVal ValueType
	wr := util.NewByteReader(expr)

	if len(expr) == 0 {
		return nil, errors.New("ErrEmptyInitExpr")
	}

	for {
		b, err := wr.ReadOne()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		switch b {
		case i32Const:
			i, err := leb128.ReadInt32(wr)
			if err != nil {
				return nil, err
			}
			stack = append(stack, uint64(i))
			lastVal = ValueTypeI32
		case i64Const:
			i, err := leb128.ReadInt64(wr)
			if err != nil {
				return nil, err
			}
			stack = append(stack, uint64(i))
			lastVal = ValueTypeI64
		case f32Const:
			b, err := wr.Read(4)
			if err != nil {
				return nil, err
			}
			i := binary.LittleEndian.Uint32(b)
			stack = append(stack, uint64(i))
			lastVal = ValueTypeF32
		case f64Const:
			b, err := wr.Read(8)
			if err != nil {
				return nil, err
			}
			i := binary.LittleEndian.Uint64(b)
			stack = append(stack, uint64(i))
			lastVal = ValueTypeF64
		case getGlobal:
			index, err := leb128.ReadUint32(wr)
			if err != nil {
				return nil, err
			}
			globalVar := m.GetGlobal(int(index))
			if globalVar == nil {
				return nil, errors.New("InvalidGlobalIndexError")
			}
			lastVal = globalVar.Type.ValueType
		case end:
			break
		default:
			return nil, errors.New("InvalidInitExprOpError")
		}
	}

	if len(stack) == 0 {
		return nil, nil
	}

	v := stack[len(stack)-1]
	switch lastVal {
	case ValueTypeI32:
		return int32(v), nil
	case ValueTypeI64:
		return int64(v), nil
	case ValueTypeF32:
		return math.Float32frombits(uint32(v)), nil
	case ValueTypeF64:
		return math.Float64frombits(uint64(v)), nil
	default:
		panic(fmt.Sprintf("Invalid value type produced by initializer expression: %d", int8(lastVal)))
	}
}

func (m *Module) populateFunctions() error {
	if m.TypeSec == nil || m.FuncSec == nil {
		return nil
	}

	for codeIndex, typeIndex := range m.FuncSec.TypeIndices {
		if int(typeIndex) >= len(m.TypeSec.FuncTypes) {
			return errors.New("Invalid function index")
		}

		// Create the main function structure
		fn := Function{
			Type: m.TypeSec.FuncTypes[typeIndex],
			Code: m.CodeSec.Codes[codeIndex],
			Name: "",
		}

		m.FunctionIndexSpace = append(m.FunctionIndexSpace, fn)
	}

	funcs := make([]uint32, 0, len(m.FuncSec.TypeIndices))
	funcs = append(funcs, m.FuncSec.TypeIndices...)
	m.FuncSec.TypeIndices = funcs
	return nil
}

func (m *Module) GetFunction(i int) *Function {
	if i >= len(m.FunctionIndexSpace) || i < 0 {
		return nil
	}

	return &m.FunctionIndexSpace[i]
}

func (m *Module) populateGlobals() error {
	if m.GlobalSec == nil {
		return nil
	}

	m.GlobalIndexSpace = append(m.GlobalIndexSpace, m.GlobalSec.Globals...)
	return nil
}

func (m *Module) GetGlobal(i int) *Global {
	if i >= len(m.GlobalIndexSpace) || i < 0 {
		return nil
	}

	return &m.GlobalIndexSpace[i]
}

func (m *Module) populateTables() error {
	if m.TableSec == nil || len(m.TableSec.Tables) == 0 || m.ElementSec == nil || len(m.ElementSec.Elements) == 0 {
		return nil
	}

	for _, elem := range m.ElementSec.Elements {
		// the MVP dictates that index should always be zero, we should
		// probably check this
		if elem.TableIdx >= uint32(len(m.TableIndexSpace)) {
			return errors.New("Invalid Table Index")
		}

		val, err := m.ExecInitExpr(elem.Init)
		if err != nil {
			return err
		}
		off, ok := val.(int32)
		if !ok {
			return errors.New("Invalid Value Type Init Expr")
		}
		offset := uint32(off)

		table := m.TableIndexSpace[elem.TableIdx]
		//use uint64 to avoid overflow
		if uint64(offset)+uint64(len(elem.Offset)) > uint64(len(table)) {
			data := make([]uint32, uint64(offset)+uint64(len(elem.Offset)))
			copy(data[offset:], elem.Offset)
			copy(data, table)
			m.TableIndexSpace[elem.TableIdx] = data
		} else {
			copy(table[offset:], elem.Offset)
		}
	}

	return nil
}

func (m *Module) GetTableElement(index int) (uint32, error) {
	if index >= len(m.TableIndexSpace[0]) {
		return 0, errors.New("Invalid table index")
	}

	return m.TableIndexSpace[0][index], nil
}

func (m *Module) populateLinearMemory() error {
	if m.DataSec == nil || len(m.DataSec.DataSegments) == 0 {
		return nil
	}
	// each module can only have a single linear memory in the MVP

	for _, entry := range m.DataSec.DataSegments {
		if entry.MemIdx != 0 {
			return errors.New("Invalid Linear Memory Index Error")
		}

		val, err := m.ExecInitExpr(entry.Offset)

		if err != nil {
			return err
		}
		off, ok := val.(int32)
		if !ok {
			return errors.New("InvalidValueTypeInitExprError")
		}
		offset := uint32(off)

		memory := m.LinearMemoryIndexSpace[entry.MemIdx]
		if uint64(offset)+uint64(len(entry.Init)) > uint64(len(memory)) {
			data := make([]byte, uint64(offset)+uint64(len(entry.Init)))
			copy(data, memory)
			copy(data[offset:], entry.Init)
			m.LinearMemoryIndexSpace[int(entry.MemIdx)] = data
		} else {
			copy(memory[offset:], entry.Init)
		}
	}

	return nil
}

func (m *Module) GetLinearMemoryData(index int) (byte, error) {
	if index >= len(m.LinearMemoryIndexSpace[0]) {
		return 0, errors.New("Invalid linear memory index")
	}

	return m.LinearMemoryIndexSpace[0][index], nil
}
