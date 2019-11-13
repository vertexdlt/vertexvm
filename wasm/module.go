package wasm

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/vertexdlt/vertexvm/leb128"
	"github.com/vertexdlt/vertexvm/util"
)

// Magic represent Wasm 4-byte magic number (the string ‘\0asm’)
const Magic uint32 = 0x6d736100

// Version represent Wasm current version
const Version uint32 = 0x1

const (
	// ValueTypeI32 represent valtype i32
	ValueTypeI32 ValueType = 0x7f
	// ValueTypeI64 represent valtype i64
	ValueTypeI64 ValueType = 0x7e
	// ValueTypeF32 represent valtype f32
	ValueTypeF32 ValueType = 0x7d
	// ValueTypeF64 represent valtype f64
	ValueTypeF64 ValueType = 0x7c
)

// BlockTypeEmpty represent empty block type
const BlockTypeEmpty uint32 = 0x40

// FuncTypeForm represent FuncType signature byte
const FuncTypeForm byte = 0x60

// ElemTypeFuncRef represent element type funcref
const ElemTypeFuncRef byte = 0x70

// ValueType represent ValueType
type ValueType int8

// Mutability represent mutability
type Mutability uint8

// Import represent the Import component
// https://webassembly.github.io/spec/core/binary/modules.html#binary-import
type Import struct {
	ModuleName string
	FieldName  string
	ImportDesc ImportDesc
}

// ImportDesc represent the Import Description
// https://webassembly.github.io/spec/core/binary/modules.html#binary-importdesc
type ImportDesc struct {
	Kind       byte
	TypeIdx    uint32
	Table      *Table
	Mem        *Mem
	GlobalType *GlobalType
}

const (
	// ExternalFunction is a type of import
	ExternalFunction byte = 0x00
	// ExternalTable is a type of import
	ExternalTable byte = 0x01
	// ExternalMemory is a type of import
	ExternalMemory byte = 0x02
	// ExternalGlobalType is a type of import
	ExternalGlobalType byte = 0x03
)

// FuncType represent Function Types
// from https://webassembly.github.io/spec/core/binary/types.html#function-types
type FuncType struct {
	ParamTypes  []ValueType
	ReturnTypes []ValueType
}

// Limits represent Limits
// from https://webassembly.github.io/spec/core/binary/types.html#limits
type Limits struct {
	Flag uint8
	Min  uint32
	Max  uint32
}

// Mem represent Memory Types
// from https://webassembly.github.io/spec/core/binary/types.html#memory-types
type Mem struct {
	Limits Limits
}

// Table represent Table Types
// from https://webassembly.github.io/spec/core/binary/types.html#table-types
type Table struct {
	ElemType byte
	Limits   Limits
}

// GlobalType represent Global Types
// from https://webassembly.github.io/spec/core/binary/types.html#global-types
type GlobalType struct {
	ValueType  ValueType
	Mutability Mutability
}

// Global represent the Global component
// according to https://webassembly.github.io/spec/core/binary/modules.html#global-section
type Global struct {
	Type GlobalType
	Init []byte
}

// ExportDesc represent Export Description https://webassembly.github.io/spec/core/binary/modules.html#binary-exportdesc
type ExportDesc struct {
	Kind byte
	Idx  uint32 // Idx can be FuncIdx | TableIdx | MemIdx | GlobalIdx
}

// Export represent the Export component
// according to https://webassembly.github.io/spec/core/binary/modules.html#export-section
type Export struct {
	Name string
	Desc ExportDesc
}

// Element represent the Element component
// https://webassembly.github.io/spec/core/binary/modules.html#binary-elem
type Element struct {
	TableIdx uint32
	Init     []byte
	Offset   []uint32 // Offset is an array of FuncIdx
}

// Code represent the code entry of the Code section
// https://webassembly.github.io/spec/core/binary/modules.html#binary-code
type Code struct {
	Size   uint32
	Locals []Local
	Exprs  []byte
}

// Data represent the data entry of the Data section
type Data struct {
	MemIdx uint32
	Offset []byte
	Init   []byte
}

// Local represent the count Locals of the same value type
// https://webassembly.github.io/spec/core/binary/modules.html#binary-local
type Local struct {
	Count     uint32
	ValueType ValueType
}

// TypeSec represent the Type Section
// https://webassembly.github.io/spec/core/binary/modules.html#type-section
type TypeSec struct {
	FuncTypes []FuncType
}

// ImportSec represent the Import Section
// https://webassembly.github.io/spec/core/binary/modules.html#binary-importsec
type ImportSec struct {
	Imports []Import
}

// FuncSec represent the Function Section
// https://webassembly.github.io/spec/core/binary/modules.html#function-section
type FuncSec struct {
	TypeIndices []uint32
}

// TableSec represent the Table Section
// https://webassembly.github.io/spec/core/binary/modules.html#function-section
type TableSec struct {
	Tables []Table
}

// MemSec represent the Memory Section
// https://webassembly.github.io/spec/core/binary/modules.html#memory-section
type MemSec struct {
	Mems []Mem
}

// GlobalSec represent the Global Section
// https://webassembly.github.io/spec/core/binary/modules.html#global-section
type GlobalSec struct {
	Globals []Global
}

// ExportSec represent the Export Section
// https://webassembly.github.io/spec/core/binary/modules.html#export-section
type ExportSec struct {
	ExportMap map[string]Export
}

// StartSec represent the Start Section
// https://webassembly.github.io/spec/core/binary/modules.html#start-section
type StartSec struct {
	FuncIdx uint32
}

// ElementSec represent the Element Section
// https://webassembly.github.io/spec/core/binary/modules.html#element-section
type ElementSec struct {
	Elements []Element
}

// CodeSec represent the Code Section
// https://webassembly.github.io/spec/core/binary/modules.html#code-section
type CodeSec struct {
	Codes []Code
}

// DataSec represent the Data Section
type DataSec struct {
	DataSegments []Data
}

// ReadModule read a module from Reader r and return a constructed Module
func ReadModule(wasmBytes []byte) (*Module, error) {
	br := util.NewByteReader(wasmBytes)
	m := &Module{}

	err := readMagic(br)
	if err != nil {
		return nil, err
	}

	err = readVersion(m, br)
	if err != nil {
		return nil, err
	}

	var lastID *byte
	for {
		lastID, err = readSection(m, br, lastID)

		if err != nil {
			if err != io.EOF {
				return nil, err
			}

			m.LinearMemoryIndexSpace = make([][]byte, 1)
			if m.TableSec != nil {
				m.TableIndexSpace = make([][]uint32, int(len(m.TableSec.Tables)))
			}

			for _, fn := range []func() error{
				m.populateGlobals,
				m.populateFunctions,
				m.populateTables,
				m.populateLinearMemory,
			} {
				if err := fn(); err != nil {
					return nil, err
				}
			}

			return m, nil
		}
	}
}

func readMagic(br *util.ByteReader) (err error) {
	b, err := br.Read(4)
	if err != nil {
		return err
	}

	magic := binary.LittleEndian.Uint32(b)
	if magic != Magic {
		return errors.New("wasm: invalid magic number")
	}
	return nil
}

func readVersion(m *Module, br *util.ByteReader) (err error) {
	b, err := br.Read(4)
	if err != nil {
		return err
	}

	m.Version = binary.LittleEndian.Uint32(b)
	if m.Version != Version {
		return errors.New("wasm: invalid version number")
	}

	return nil
}

func readSection(m *Module, br *util.ByteReader, lastID *byte) (*byte, error) {
	id, err := br.ReadOne()
	if err != nil {
		return nil, err
	}

	if lastID != nil && *lastID != 0 {
		if *lastID >= id && id != 0 {
			return nil, fmt.Errorf("wasm: sections must occur at most once and in the prescribed order")
		}
	}

	datalen, err := leb128.ReadUint32(br)
	if err != nil {
		return nil, err
	}

	b, err := br.Read(datalen)
	if err != nil {
		return nil, err
	}
	sectionReader := util.NewByteReader(b)
	// fmt.Println(id)
	// fmt.Printf("%s", hex.Dump(sectionReader.b))

	switch id {
	case 0:
		// Skip custom section
	case 1:
		err := readSectionType(m, sectionReader)
		if err != nil {
			return nil, err
		}
	case 2:
		err := readSectionImport(m, sectionReader)
		if err != nil {
			return nil, err
		}
	case 3:
		err := readSectionFunction(m, sectionReader)
		if err != nil {
			return nil, err
		}
	case 4:
		err := readSectionTable(m, sectionReader)
		if err != nil {
			return nil, err
		}
	case 5:
		err := readSectionMemory(m, sectionReader)
		if err != nil {
			return nil, err
		}
	case 6:
		err := readSectionGlobal(m, sectionReader)
		if err != nil {
			return nil, err
		}
	case 7:
		err := readSectionExport(m, sectionReader)
		if err != nil {
			return nil, err
		}
	case 8:
		err := readSectionStart(m, sectionReader)
		if err != nil {
			return nil, err
		}
	case 9:
		err := readSectionElement(m, sectionReader)
		if err != nil {
			return nil, err
		}
	case 10:
		err := readSectionCode(m, sectionReader)
		if err != nil {
			return nil, err
		}
	case 11:
		err := readSectionData(m, sectionReader)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("wasm: read section error - unknown section id %d", id)
	}

	return &id, err
}

func readSectionType(m *Module, br *util.ByteReader) error {
	vectorLen, err := leb128.ReadUint32(br)
	if err != nil {
		return err
	}

	m.TypeSec = &TypeSec{}
	m.TypeSec.FuncTypes = make([]FuncType, vectorLen)
	for i := uint32(0); i < vectorLen; i++ {
		funcTypeForm, err := br.ReadOne()
		if err != nil {
			return err
		}
		if funcTypeForm != FuncTypeForm {
			return errors.New("wasm: invalid functype signature byte")
		}

		paramTypesCount, err := leb128.ReadUint32(br)
		if err != nil {
			return err
		}

		m.TypeSec.FuncTypes[i].ParamTypes = make([]ValueType, paramTypesCount)
		for j := uint32(0); j < paramTypesCount; j++ {
			m.TypeSec.FuncTypes[i].ParamTypes[j], err = readValueType(br)
			if err != nil {
				return err
			}
		}

		returnTypesCount, err := leb128.ReadUint32(br)
		if err != nil {
			return err
		}

		m.TypeSec.FuncTypes[i].ReturnTypes = make([]ValueType, returnTypesCount)
		for j := uint32(0); j < returnTypesCount; j++ {
			m.TypeSec.FuncTypes[i].ReturnTypes[j], err = readValueType(br)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func readSectionImport(m *Module, br *util.ByteReader) error {
	importCount, err := leb128.ReadUint32(br)
	if err != nil {
		return err
	}

	m.ImportSec = &ImportSec{}
	m.ImportSec.Imports = make([]Import, importCount)
	for i := uint32(0); i < importCount; i++ {
		m.ImportSec.Imports[i].ModuleName, err = readName(br)
		if err != nil {
			return err
		}

		m.ImportSec.Imports[i].FieldName, err = readName(br)
		if err != nil {
			return err
		}

		kind, err := br.ReadOne()
		if err != nil {
			return err
		}

		var importDesc ImportDesc
		switch kind {
		case ExternalFunction:
			importDesc.TypeIdx, err = leb128.ReadUint32(br)
			if err != nil {
				return err
			}
		case ExternalTable:
			importDesc.Table = &Table{}
			importDesc.Table.ElemType, err = readElemType(br)
			if err != nil {
				return err
			}

			importDesc.Table.Limits, err = readLimits(br)
			if err != nil {
				return err
			}
		case ExternalMemory:
			importDesc.Mem = &Mem{}
			importDesc.Mem.Limits, err = readLimits(br)
			if err != nil {
				return err
			}
		case ExternalGlobalType:
			globalType, err := readGlobalType(br)
			if err != nil {
				return err
			}
			importDesc.GlobalType = &globalType
		default:
			return fmt.Errorf("wasm: invalid external kind %v", kind)
		}

		importDesc.Kind = kind
		m.ImportSec.Imports[i].ImportDesc = importDesc
	}
	return nil
}

func readSectionFunction(m *Module, br *util.ByteReader) error {
	typeIdxCount, err := leb128.ReadUint32(br)
	if err != nil {
		return err
	}

	m.FuncSec = &FuncSec{}
	m.FuncSec.TypeIndices = make([]uint32, typeIdxCount)
	for i := uint32(0); i < typeIdxCount; i++ {
		m.FuncSec.TypeIndices[i], err = leb128.ReadUint32(br)
		if err != nil {
			return err
		}
	}
	return nil
}

func readSectionTable(m *Module, br *util.ByteReader) error {
	tableCount, err := leb128.ReadUint32(br)
	if err != nil {
		return err
	}

	m.TableSec = &TableSec{}
	m.TableSec.Tables = make([]Table, tableCount)
	for i := uint32(0); i < tableCount; i++ {
		m.TableSec.Tables[i].ElemType, err = readElemType(br)
		if err != nil {
			return err
		}

		m.TableSec.Tables[i].Limits, err = readLimits(br)
		if err != nil {
			return err
		}
	}

	return nil
}

func readSectionMemory(m *Module, br *util.ByteReader) error {
	memCount, err := leb128.ReadUint32(br)
	if err != nil {
		return err
	}

	m.MemSec = &MemSec{}
	m.MemSec.Mems = make([]Mem, memCount)
	for i := uint32(0); i < memCount; i++ {
		m.MemSec.Mems[i].Limits, err = readLimits(br)
		if err != nil {
			return err
		}
	}

	return nil
}

func readSectionGlobal(m *Module, br *util.ByteReader) error {
	globalCount, err := leb128.ReadUint32(br)
	if err != nil {
		return err
	}

	m.GlobalSec = &GlobalSec{}
	m.GlobalSec.Globals = make([]Global, globalCount)
	for i := uint32(0); i < globalCount; i++ {
		m.GlobalSec.Globals[i].Type, err = readGlobalType(br)
		if err != nil {
			return err
		}

		m.GlobalSec.Globals[i].Init, err = readExprs(br)
		if err != nil {
			return err
		}
	}

	return nil
}

func readSectionExport(m *Module, br *util.ByteReader) error {
	exportCount, err := leb128.ReadUint32(br)
	if err != nil {
		return err
	}

	m.ExportSec = &ExportSec{}
	m.ExportSec.ExportMap = make(map[string]Export, exportCount)
	for i := uint32(0); i < exportCount; i++ {
		var export Export
		export.Name, err = readName(br)
		if err != nil {
			return err
		}

		b, err := br.ReadOne()
		if err != nil {
			return err
		}
		if b != 0x00 && b != 0x01 && b != 0x02 && b != 0x03 {
			return errors.New("wasm: invalid export desc flag")
		}

		export.Desc.Kind = b
		export.Desc.Idx, err = leb128.ReadUint32(br)
		if err != nil {
			return err
		}

		m.ExportSec.ExportMap[export.Name] = export
	}

	return nil
}

func readSectionStart(m *Module, br *util.ByteReader) error {
	var err error
	m.StartSec = &StartSec{}
	m.StartSec.FuncIdx, err = leb128.ReadUint32(br)
	return err
}

func readSectionElement(m *Module, br *util.ByteReader) error {
	elementCount, err := leb128.ReadUint32(br)
	if err != nil {
		return err
	}

	m.ElementSec = &ElementSec{}
	m.ElementSec.Elements = make([]Element, elementCount)
	for i := uint32(0); i < elementCount; i++ {
		m.ElementSec.Elements[i].TableIdx, err = leb128.ReadUint32(br)
		if err != nil {
			return err
		}

		m.ElementSec.Elements[i].Init, err = readExprs(br)
		if err != nil {
			return err
		}

		funcIdxCount, err := leb128.ReadUint32(br)
		if err != nil {
			return err
		}

		funcIdxes := make([]uint32, funcIdxCount)
		for j := uint32(0); j < funcIdxCount; j++ {
			funcIdxes[j], err = leb128.ReadUint32(br)
			if err != nil {
				return err
			}
		}
		m.ElementSec.Elements[i].Offset = funcIdxes
	}

	return nil
}

func readSectionCode(m *Module, br *util.ByteReader) error {
	codeCount, err := leb128.ReadUint32(br)
	if err != nil {
		return err
	}

	m.CodeSec = &CodeSec{}
	m.CodeSec.Codes = make([]Code, codeCount)
	for i := uint32(0); i < codeCount; i++ {
		size, err := leb128.ReadUint32(br)
		if err != nil {
			return err
		}

		codeBody, err := br.Read(size)
		if err != nil {
			return err
		}

		code := util.NewByteReader(codeBody)
		m.CodeSec.Codes[i].Locals, err = readLocals(code)
		if err != nil {
			return err
		}

		exprs := code.CopyAll()
		m.CodeSec.Codes[i].Exprs = exprs[:len(exprs)-1]
		m.CodeSec.Codes[i].Size = size
	}

	return nil
}

func readSectionData(m *Module, br *util.ByteReader) error {
	dataCount, err := leb128.ReadUint32(br)
	if err != nil {
		return err
	}

	m.DataSec = &DataSec{}
	m.DataSec.DataSegments = make([]Data, dataCount)
	for i := uint32(0); i < dataCount; i++ {
		m.DataSec.DataSegments[i].MemIdx, err = leb128.ReadUint32(br)
		if err != nil {
			return err
		}

		m.DataSec.DataSegments[i].Offset, err = readExprs(br)
		if err != nil {
			return err
		}

		byteCount, err := leb128.ReadUint32(br)
		if err != nil {
			return err
		}

		m.DataSec.DataSegments[i].Init, err = br.Read(byteCount)
	}
	return nil
}

func readElemType(br *util.ByteReader) (byte, error) {
	var elemType byte
	elemType, err := br.ReadOne()
	if err != nil {
		return elemType, err
	}

	// Version 1 of WebAssembly only support funcref
	// https://webassembly.github.io/spec/core/syntax/types.html#syntax-elemtype
	if elemType != ElemTypeFuncRef {
		return elemType, errors.New("wasm: invalid table element type")
	}

	return elemType, nil
}

func readLimits(br *util.ByteReader) (Limits, error) {
	var (
		limits Limits
		err    error
	)
	limits.Flag, err = br.ReadOne()
	if err != nil {
		return limits, err
	}

	switch limits.Flag {
	case 0x00:
		limits.Min, err = leb128.ReadUint32(br)
		if err != nil {
			return limits, err
		}
	case 0x01:
		limits.Min, err = leb128.ReadUint32(br)
		if err != nil {
			return limits, err
		}
		limits.Max, err = leb128.ReadUint32(br)
		if err != nil {
			return limits, err
		}
	default:
		return limits, errors.New("wasm: invalid limits flag")
	}

	return limits, nil
}

func readGlobalType(br *util.ByteReader) (GlobalType, error) {
	var (
		globalType GlobalType
		err        error
	)

	globalType.ValueType, err = readValueType(br)
	if err != nil {
		return globalType, err
	}

	globalType.Mutability, err = readMut(br)
	if err != nil {
		return globalType, err
	}

	return globalType, nil
}

func readMut(br *util.ByteReader) (Mutability, error) {
	var res Mutability
	b, err := br.ReadOne()
	if err != nil {
		return res, err
	}
	if b != 0x00 && b != 0x01 {
		return res, errors.New("wasm: invalid mutability flag")
	}

	res = Mutability(b)
	return res, nil
}

func readValueType(br *util.ByteReader) (ValueType, error) {
	var res ValueType
	b, err := br.ReadOne()
	if err != nil {
		return res, err
	}
	if b != 0x7F && b != 0x7E && b != 0x7D && b != 0x7C {
		return res, errors.New("wasm: invalid value type")
	}
	res = ValueType(b)
	return res, nil
}

func readName(br *util.ByteReader) (string, error) {
	byteLen, err := leb128.ReadUint32(br)
	if err != nil {
		return "", err
	}

	bytes, err := br.Read(byteLen)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(bytes) {
		return "", errors.New("wasm: invalid utf-8 string")
	}
	return string(bytes), nil
}

func readLocals(br *util.ByteReader) ([]Local, error) {
	localCount, err := leb128.ReadUint32(br)
	if err != nil {
		return []Local{}, err
	}

	locals := make([]Local, localCount)
	for i := uint32(0); i < localCount; i++ {
		locals[i].Count, err = leb128.ReadUint32(br)
		if err != nil {
			return locals, err
		}

		locals[i].ValueType, err = readValueType(br)
		if err != nil {
			return locals, err
		}
	}

	return locals, nil
}

func readExprs(br *util.ByteReader) ([]byte, error) {
	var (
		opcode byte
		exprs  []byte
		err    error
	)
	for opcode != 0x0B {
		opcode, err = br.ReadOne()
		if err != nil {
			return exprs, err
		}
		exprs = append(exprs, opcode)
	}

	return exprs, nil
}
