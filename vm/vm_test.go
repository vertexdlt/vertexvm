package vm

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os/exec"
	"strconv"
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
	name     string
	params   []uint64
	expected uint64
}

func TestVM(t *testing.T) {
	tests := []vmTest{
		{name: "i32", params: []uint64{}, expected: 0},
		{name: "local", params: []uint64{2}, expected: 3},
		{name: "call", params: []uint64{}, expected: 16},
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
		vm, err := NewVM(data)
		if err != nil {
			panic(err)
		}
		fnID, ok := vm.GetFunctionIndex("calc")
		if !ok {
			t.Error("cannot get function export")
		}
		ret := vm.Invoke(fnID, test.params...)
		if ret != test.expected {
			t.Errorf("Expect return value to be %d, got %d", test.expected, ret)
		}
	}
}

func TestWasmSuite(t *testing.T) {
	tests := []string{"i32", "i64"}
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
			switch cmd.Type {
			case "module":
				data, err := ioutil.ReadFile(fmt.Sprintf("./test_suite/%s", cmd.Filename))
				if err != nil {
					panic(err)
				}
				vm, err = NewVM(data)
				if err != nil {
					panic(err)
				}
			case "assert_return", "action":
				switch cmd.Action.Type {
				case "invoke":
					funcID, ok := vm.GetFunctionIndex(cmd.Action.Field)
					if !ok {
						panic("function not found")
					}
					args := make([]uint64, 0)
					for _, arg := range cmd.Action.Args {
						val, err := strconv.ParseUint(arg.Value, 10, 64)
						if err != nil {
							panic(err)
						}
						args = append(args, val)
					}
					t.Logf("Triggering %s with args at line %d", cmd.Action.Field, cmd.Line)
					t.Log(args)
					ret := vm.Invoke(funcID, args...)
					t.Log("ret", ret)

					if len(cmd.Expected) != 0 {
						exp, err := strconv.ParseUint(cmd.Expected[0].Value, 10, 64)
						if err != nil {
							panic(err)
						}

						if cmd.Expected[0].Type == "i32" {
							ret = uint64(uint32(ret))
							exp = uint64(uint32(exp))
						}

						if ret != exp {
							t.Errorf("Expect return value to be %d, got %d", exp, ret)
						}
					}
				default:
					t.Errorf("unknown action %s", cmd.Action.Type)
				}
			case "assert_trap", "assert_invalid":
				t.Logf("%s not supported", cmd.Type)
			default:
				t.Errorf("unknown command %s", cmd.Type)
			}
		}
	}
}
