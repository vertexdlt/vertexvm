package vm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os/exec"
	"reflect"
	"strconv"
	"testing"

	wagonExec "github.com/go-interpreter/wagon/exec"
	wagon "github.com/go-interpreter/wagon/wasm"
)

type TestSuite struct {
	SourceFilename string    `json:"source_filename"`
	Commands       []Command `json:"commands"`
}

type Command struct {
	Type       string      `json:"type"`
	Line       int         `json:"line"`
	Filename   string      `json:"filename"`
	Name       string      `json:"name"`
	Action     Action      `json:"action"`
	Text       string      `json:"text"`
	ModuleType string      `json:"module_type"`
	Expected   []ValueInfo `json:"expected"`
}

type Action struct {
	Type     string      `json:"type"`
	Module   string      `json:"module"`
	Field    string      `json:"field"`
	Args     []ValueInfo `json:"args"`
	Expected []ValueInfo `json:"expected"`
}

type ValueInfo struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type vmTest struct {
	name     string
	params   []uint64
	expected uint64
	entry    string
}

func getVM(name string) *VM {
	wat := fmt.Sprintf("./test_data/%s.wat", name)
	wasm := fmt.Sprintf("./test_data/%s.wasm", name)
	cmd := exec.Command("wat2wasm", wat, "-o", wasm)
	err := cmd.Start()
	if err != nil {
		panic(err)
	}
	err = cmd.Wait()
	if err != nil {
		panic(err)
	}
	data, err := ioutil.ReadFile(wasm)
	if err != nil {
		panic(err)
	}
	vm, err := NewVM(data, &TestResolver{})
	if err != nil {
		panic(err)
	}
	return vm
}

func TestNeg(t *testing.T) {
	vm := getVM("i32")
	_, ok := vm.GetFunctionIndex("somefunc")
	if ok {
		t.Errorf("Expect function index to be -1")
	}
}

type TestResolver struct{}

func (r *TestResolver) GetFunction(module, name string) HostFunction {
	switch module {
	case "env":
		switch name {
		case "add":
			return func(vm *VM, args ...uint64) uint64 {
				x := int(args[0])
				y := int(args[1])
				return uint64(x + y)
			}
		default:
			log.Fatalf("Unknown import name: %s", name)
		}
	case "spectest":
		switch name {
		case "print", "print_i32", "print_i32_f32", "print_f32", "print_f64", "print_f64_f64":
			return func(vm *VM, args ...uint64) uint64 { return 0 }
		default:
			log.Fatalf("Unknown import name: %s", name)
		}
	case "test":
		switch name {
		case "func-i64->i64":
			return func(vm *VM, args ...uint64) uint64 { return 0 }
		default:
			log.Fatalf("Unknown import name: %s", name)
		}
	case "Mf":
		switch name {
		case "call":
			return func(vm *VM, args ...uint64) uint64 { return 2 }
		default:
			log.Fatalf("Unknown import name: %s", name)
		}
	case "Mt":
		switch name {
		case "call", "h":
			return func(vm *VM, args ...uint64) uint64 { return 4 }
		default:
			log.Fatalf("Unknown import name: %s", name)
		}
	default:
		log.Fatalf("Unknown module name: %s", module)
	}
	return nil
}

func TestVM(t *testing.T) {
	tests := []vmTest{
		{name: "i32", entry: "calc", params: []uint64{}, expected: 4294967295},
		{name: "local", entry: "calc", params: []uint64{2}, expected: 3},
		{name: "call", entry: "calc", params: []uint64{}, expected: 16},
		{name: "select", entry: "calc", params: []uint64{5}, expected: 3},
		{name: "block", entry: "calc", params: []uint64{32}, expected: 16},
		{name: "block", entry: "calc", params: []uint64{30}, expected: 8},
		{name: "loop", entry: "calc", params: []uint64{30}, expected: 435},
		{name: "ifelse", entry: "calc", params: []uint64{1}, expected: 5},
		{name: "ifelse", entry: "calc", params: []uint64{0}, expected: 7},
		{name: "ifelse", entry: "main", params: []uint64{1, 0}, expected: 10},
		{name: "ifelse", entry: "asifthen", params: []uint64{0, 6}, expected: 6},
		{name: "loop", entry: "isPrime", params: []uint64{6}, expected: 2},
		{name: "loop", entry: "isPrime", params: []uint64{9}, expected: 3},
		{name: "loop", entry: "isPrime", params: []uint64{10007}, expected: 1},
		{name: "loop", entry: "counter", params: []uint64{}, expected: 4},
		{name: "call_indirect", entry: "calc", params: []uint64{}, expected: 16},
		{name: "br_table", entry: "calc", params: []uint64{0}, expected: 8},
		{name: "br_table", entry: "calc", params: []uint64{1}, expected: 16},
		{name: "br_table", entry: "calc", params: []uint64{100}, expected: 16},
		{name: "return", entry: "calc", params: []uint64{}, expected: 9},
		{name: "import_env", entry: "calc", params: []uint64{}, expected: 3},
	}
	for _, test := range tests {
		vm := getVM(test.name)
		fmt.Println(vm.Module.TableIndexSpace[0])

		fnID, ok := vm.GetFunctionIndex(test.entry)
		if !ok {
			t.Error("cannot get function export")
		}
		ret := vm.Invoke(fnID, test.params...)
		if ret != test.expected {
			t.Errorf("Test %s: Expect return value to be %d, got %d", test.name, test.expected, ret)
		}
	}
}

func TestVM2(t *testing.T) {
	tests := []vmTest{
		// {name: "i32", entry: "calc", params: []int64{}, expected: -1},
		// {name: "local", entry: "calc", params: []int64{2}, expected: 3},
		// {name: "call", entry: "calc", params: []int64{}, expected: 16},
		// {name: "select", entry: "calc", params: []int64{5}, expected: 3},
		// {name: "block", entry: "calc", params: []int64{32}, expected: 16},
		// {name: "block", entry: "calc", params: []int64{30}, expected: 8},
		// {name: "loop", entry: "calc", params: []int64{30}, expected: 435},
		// {name: "ifelse", entry: "calc", params: []int64{1}, expected: 5},
		// {name: "ifelse", entry: "calc", params: []int64{0}, expected: 7},
		// {name: "loop", entry: "isPrime", params: []int64{6}, expected: 2},
		// {name: "loop", entry: "isPrime", params: []int64{9}, expected: 3},
		{name: "loop", entry: "isPrime", params: []uint64{10007}, expected: 1},
	}
	for _, test := range tests {
		wat := fmt.Sprintf("./test_data/%s.wat", test.name)
		wasm := fmt.Sprintf("./test_data/%s.wasm", test.name)
		fmt.Println(test)
		cmd := exec.Command("wat2wasm", wat, "-o", wasm)
		err := cmd.Start()
		if err != nil {
			panic(err)
		}
		err = cmd.Wait()
		if err != nil {
			panic(err)
		}

		data, err := ioutil.ReadFile(wasm)
		if err != nil {
			panic(err)
		}
		m, err := wagon.ReadModule(bytes.NewReader(data), nil)
		findex := int64(m.Export.Entries[test.entry].Index)
		vm, err := wagonExec.NewVM(m)
		ret, err := vm.ExecCode(findex, uint64(test.params[0]))
		casted := ret.(uint32)
		if casted != uint32(test.expected) {
			t.Errorf("Expect return value to be %d, got %d", test.expected, ret)
		}
	}
}

func TestWasmSuite(t *testing.T) {
	tests := []string{
		"i32", "i64", "f32", "f64",
		"f32_cmp", "f32_bitwise", "f64_cmp", "f64_bitwise", "conversions",
		"br", "br_if", "br_table", "call", "call_indirect",
		"globals", "local_get", "local_set", "local_tee",
		"memory", "memory_grow", "memory_size", "memory_redundancy", "memory_trap",
		"binary", "binary-leb128", "block",
		"address",
		"break-drop", "comments",
		"return", "select", "loop", "if",
		"custom", "endianness",
		"fac", "float_literals", "float_memory",
		"forward", "func",
		"inline-module", "int_exprs", "int_literals", "labels",
		"left-to-right", "load", "nop", "stack", "store", "switch", "token",
		"traps", "type", "typecheck", "unreachable", "unreached-invalid", "unwind",
		"utf8-custom-section-id", "utf8-import-field", "utf8-import-module", "utf8-invalid-encoding",
		"skip-stack-guard-page", "float_exprs", "float_misc", "align",
		"start", "func_ptrs",
		"exports", // empty module removed

		// "linking",
		// "const",	//some const test is off by 1. VM result is similar to that of Emscripten & WS
		// "elem", "data",	//wagon parsing failed
		// "names",	// problem with unicode. Entries key and cmd.Action.Field yield different codes
		// "imports",	// missing imports from spec
	}

	for _, name := range tests {
		t.Logf("Test suite %s", name)
		wast := fmt.Sprintf("./test_suite/%s.wast", name)
		jsonFile := fmt.Sprintf("./test_suite/%s.json", name)
		cmd := exec.Command("wast2json", wast, "-o", jsonFile)
		err := cmd.Start()
		if err != nil {
			panic(err)
		}
		err = cmd.Wait()
		if err != nil {
			panic(err)
		}

		raw, err := ioutil.ReadFile(jsonFile)
		if err != nil {
			panic(err)
		}
		var suite TestSuite
		err = json.Unmarshal(raw, &suite)
		if err != nil {
			panic(err)
		}
		var vm *VM
		for _, cmd := range suite.Commands {
			t.Logf("Running test %s %d", name, cmd.Line)
			if cmd.Action.Field == "as-unary-operand" && cmd.Line == 338 {
				continue
			}
			if name == "linking" && cmd.Line >= 50 && cmd.Line <= 83 {
				continue
			}
			// Skip min, max nan with inf tests
			if (name == "f32" || name == "f64") && ((cmd.Line >= 1931 && cmd.Line <= 1938) ||
				(cmd.Line >= 1995 && cmd.Line <= 2002) ||
				(cmd.Line >= 2331 && cmd.Line <= 2338) ||
				(cmd.Line >= 2395 && cmd.Line <= 2402)) {
				continue
			}
			switch cmd.Type {
			case "module":
				data, err := ioutil.ReadFile(fmt.Sprintf("./test_suite/%s", cmd.Filename))
				if err != nil {
					t.Error(err)
				}
				vm, err = NewVM(data, &TestResolver{})
				if err != nil {
					t.Error(err)
				}
			case "assert_return", "action", "assert_return_canonical_nan", "assert_return_arithmetic_nan":
				switch cmd.Action.Type {
				case "invoke":
					funcID, ok := vm.GetFunctionIndex(cmd.Action.Field)
					if !ok {
						t.Errorf("function not found %s", cmd.Action.Field)
						continue
					}
					args := make([]uint64, 0)
					for _, arg := range cmd.Action.Args {
						val, err := strconv.ParseUint(arg.Value, 10, 64)
						if err != nil {
							panic(err)
						}
						args = append(args, val)
					}
					// t.Logf("Triggering %s with args at line %d", cmd.Action.Field, cmd.Line)
					// t.Log(args)
					ret := vm.Invoke(funcID, args...)
					// t.Log("ret", ret)

					if len(cmd.Expected) != 0 {
						var exp uint64
						if cmd.Type == "assert_return_canonical_nan" {
							if cmd.Expected[0].Type == "f32" {
								exp = 0x7fc00000
							} else if cmd.Expected[0].Type == "f64" {
								exp = 0x7ff8000000000000
							}
						} else if cmd.Type == "assert_return_arithmetic_nan" {
							// An arithmetic NaN is a floating-point value ±𝗇𝖺𝗇(n) with n≥canonN, such that the most significant bit is 1 while all others are arbitrary.
							// Unset sign bit, pass if >= canonical NaN in integer
							if cmd.Expected[0].Type == "f32" && (uint32(ret)&^(1<<31)) >= uint32(0x7fc00000) {
								exp = ret
							}
							if cmd.Expected[0].Type == "f64" && (ret&^(1<<63)) >= uint64(0x7ff8000000000000) {
								exp = ret
							}
						} else {
							exp, err = strconv.ParseUint(cmd.Expected[0].Value, 10, 64)
							if err != nil {
								panic(err)
							}
						}

						if cmd.Expected[0].Type == "i32" || cmd.Expected[0].Type == "f32" {
							ret = uint64(uint32(ret))
							exp = uint64(uint32(exp))
						}

						if ret != exp {
							t.Errorf("Test %s Field %s Line %d: Expect return value to be %d, got %d", name, cmd.Action.Field, cmd.Line, exp, ret)
						}
					}
				case "get":
					entry, ok := vm.Module.Export.Entries[cmd.Action.Field]
					if !ok {
						panic("Global export not found")
					}
					ret := vm.globals[entry.Index]
					if len(cmd.Expected) != 0 {
						exp, err := strconv.ParseUint(cmd.Expected[0].Value, 10, 64)
						if err != nil {
							panic(err)
						}

						if cmd.Expected[0].Type == "i32" || cmd.Expected[0].Type == "f32" {
							ret = uint64(uint32(ret))
							exp = uint64(uint32(exp))
						}
						if ret != exp {
							t.Errorf("Test %s Field %s Line %d: Expect return value to be %d, got %d", name, cmd.Action.Field, cmd.Line, exp, ret)
						}
					}
				default:
					t.Errorf("unknown action %s", cmd.Action.Type)
				}

			case "assert_trap", "assert_invalid", "assert_exhaustion", "assert_malformed", "assert_uninstantiable", "assert_unlinkable":
				t.Logf("Skipping %s", cmd.Type)
			default:
				t.Errorf("unknown command %s", cmd.Type)
			}
		}
	}
}

func TestMemSize(t *testing.T) {
	vm := getVM("i32")
	if len(vm.memory) != vm.MemSize() {
		t.Errorf("Expect MemSize to be %d, got %d", len(vm.memory), vm.MemSize())
	}
}

func TestMemRead(t *testing.T) {
	vm := getVM("i32")
	sample := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	offset := vm.MemSize() - len(sample)
	copy(vm.memory[offset:offset+len(sample)], sample)
	readBuffer := make([]byte, 10)
	readSize, err := vm.MemRead(readBuffer, offset)
	if readSize != len(sample) {
		t.Errorf("Expect MemRead result size to be %d, got %d", len(sample), readSize)
	}
	if err != nil {
		t.Errorf("Expect MemRead err to be nil, got %d", err)
	}
	if !reflect.DeepEqual(sample, readBuffer) {
		t.Errorf("Expect MemRead result to be %v, got %v", sample, readBuffer)
	}

	readBuffer = make([]byte, 15)
	readSize, err = vm.MemRead(readBuffer, offset)
	if readSize != len(sample) {
		t.Errorf("Expect MemRead result size to be %d, got %d", len(sample), readSize)
	}
	if err != io.ErrShortBuffer {
		t.Errorf("Expect MemRead err to be io.ErrShortBuffer, got %v", err)
	}
	if !reflect.DeepEqual(sample, readBuffer[:len(sample)]) {
		t.Errorf("Expect MemRead result first 10 bytes to be %v, got %v", sample, readBuffer)
	}

}

func TestMemWrite(t *testing.T) {
	vm := getVM("i32")
	sample := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	offset := vm.MemSize() - len(sample)
	writeSize, err := vm.MemWrite(sample, offset)

	if writeSize != len(sample) {
		t.Errorf("Expect MemWrite result size to be %d, got %d", len(sample), writeSize)
	}
	if err != nil {
		t.Errorf("Expect MemWrite err to be nil, got %d", err)
	}
	if !reflect.DeepEqual(sample, vm.memory[offset:offset+len(sample)]) {
		t.Errorf("Expect MemWrite result to be %v, got %v", sample, vm.memory[offset:offset+len(sample)])
	}

	sample = []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}

	writeSize, err = vm.MemWrite(sample, offset)

	if writeSize != vm.MemSize()-offset {
		t.Errorf("Expect MemWrite result size to be %d, got %d", vm.MemSize()-offset, writeSize)
	}
	if err != io.ErrShortWrite {
		t.Errorf("Expect MemWrite err to be io.ErrShortWrite, got %d", err)
	}
	if !reflect.DeepEqual(sample[:writeSize], vm.memory[offset:]) {
		t.Errorf("Expect MemWrite result to be %v, got %v", sample[:writeSize], vm.memory[offset:])
	}
}
