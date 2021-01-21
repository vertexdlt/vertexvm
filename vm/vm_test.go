package vm

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os/exec"
	"testing"
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
	name        string
	params      []uint64
	expected    uint64
	entry       string
	expectedErr error
	trapText    string
}

type TestResolver struct{}

func (r *TestResolver) GetFunction(module, name string) HostFunction {
	switch module {
	case "env":
		switch name {
		case "add":
			return func(vm *VM, args ...uint64) (uint64, error) {
				x := int(args[0])
				y := int(args[1])
				return uint64(x + y), nil
			}
		default:
			log.Fatalf("Unknown import name: %s", name)
		}
	case "spectest":
		switch name {
		case "print", "print_i32", "print_i32_f32", "print_f32", "print_f64", "print_f64_f64":
			return func(vm *VM, args ...uint64) (uint64, error) { return 0, nil }
		default:
			log.Fatalf("Unknown import name: %s", name)
		}
	case "test":
		switch name {
		case "func-i64->i64":
			return func(vm *VM, args ...uint64) (uint64, error) { return 0, nil }
		default:
			log.Fatalf("Unknown import name: %s", name)
		}
	case "Mf":
		switch name {
		case "call":
			return func(vm *VM, args ...uint64) (uint64, error) { return 2, nil }
		default:
			log.Fatalf("Unknown import name: %s", name)
		}
	case "Mt":
		switch name {
		case "call", "h":
			return func(vm *VM, args ...uint64) (uint64, error) { return 4, nil }
		default:
			log.Fatalf("Unknown import name: %s", name)
		}
	case "wasi_unstable":
		return func(vm *VM, args ...uint64) (uint64, error) {
			return 52, nil // __WASI_ENOSYS
		}
	default:
		log.Fatalf("Unknown module name: %s", module)
	}
	return nil
}

func GetTestVM(name string, gasPolicy GasPolicy, gasLimit uint64) *VM {
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
	vm, err := NewVM(data, gasPolicy, &Gas{Limit: gasLimit}, &TestResolver{})
	if err != nil {
		panic(err)
	}
	return vm
}

func TestGetFunctionIndex(t *testing.T) {
	vm := GetTestVM("i32", &FreeGasPolicy{}, 0)
	_, ok := vm.GetFunctionIndex("somefunc")
	if ok {
		t.Errorf("Expect function index to be -1")
	}
}

func TestVMError(t *testing.T) {
	tests := []vmTest{
		{name: "exit", entry: "calc", params: []uint64{1}, expectedErr: ErrUnreachable},
		{name: "local", entry: "calc", params: []uint64{}, expectedErr: ErrWrongNumberOfArgs},
		{name: "mem_access", entry: "failed_access", params: []uint64{}, expectedErr: ErrOutOfBoundMemoryAccess},
		{name: "mem_access", entry: "access", params: []uint64{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					var err error
					switch x := r.(type) {
					case string:
						err = errors.New(x)
					case error:
						err = x
					default:
						// Fallback err (per specs, error strings should be lowercase w/o punctuation
						err = errors.New("unknown panic")
					}

					if err != test.expectedErr {
						t.Errorf("Test %s: Expect return value to be %s, got %s", test.name, test.expectedErr, r)
					}
				}
			}()

			vm := GetTestVM(test.name, &FreeGasPolicy{}, 0)
			fnID, ok := vm.GetFunctionIndex(test.entry)
			if !ok {
				t.Error("cannot get function export")
			}
			_, err := vm.Invoke(fnID, test.params...)
			if err != test.expectedErr {
				t.Errorf("Test %s: Expect return value to be %s, got %s", test.name, test.expectedErr, err.Error())
			}
		})
	}
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
		{name: "trunc", entry: "main", params: []uint64{}, expected: 4294967295},
		{name: "trunc_trap", entry: "main", params: []uint64{}, trapText: "integer overflow"},
		{name: "trunc_edge", entry: "main", params: []uint64{}, expected: 0},
	}
	for _, test := range tests {
		vm := GetTestVM(test.name, &FreeGasPolicy{}, 0)

		fnID, ok := vm.GetFunctionIndex(test.entry)
		if !ok {
			t.Error("cannot get function export")
		}
		ret, err := vm.Invoke(fnID, test.params...)
		if err != nil {
			if test.trapText == "" {
				t.Errorf("Test %s: Expect no trap got %s", test.name, err.Error())
			} else if err.Error() != test.trapText {
				t.Errorf("Test %s: Expect trap text to be %s, got %s", test.name, test.trapText, err.Error())
			}
		} else if ret != test.expected {
			t.Errorf("Test %s: Expect return value to be %d, got %d", test.name, test.expected, ret)
		}
	}
}

func TestEnoughGas(t *testing.T) {
	vm := GetTestVM("i32", &SimpleGasPolicy{}, 2148)
	fnIndex, ok := vm.GetFunctionIndex("calc")
	if !ok {
		panic("Cannot get export fn index")
	}
	_, err := vm.Invoke(fnIndex)
	if err != nil {
		t.Errorf("Expect execution to go through, got %v", err)
	}
}

func TestOutOfGas(t *testing.T) {
	vm := GetTestVM("i32", &SimpleGasPolicy{}, 2058)
	fnIndex, ok := vm.GetFunctionIndex("calc")
	if !ok {
		panic("Cannot get export fn index")
	}
	_, err := vm.Invoke(fnIndex)
	if err != ErrOutOfGas {
		t.Errorf("Expect execution to be out of gas, got %v", err)
	}
}

func TestOutOfGasVMCreation(t *testing.T) {
	GetTestVM("i32", &FreeGasPolicy{}, 0)
	data, err := ioutil.ReadFile("./test_data/i32.wasm")
	if err != nil {
		panic(err)
	}
	_, err = NewVM(data, &FreeGasPolicy{}, &Gas{Limit: 10, Used: 20}, &TestResolver{})
	if err == nil || err != ErrOutOfGas {
		t.Errorf("Expect out of gas error: %d", err)
	}
}
