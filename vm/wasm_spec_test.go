package vm

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os/exec"
	"strconv"
	"testing"
)

func invokeWithAction(vm *VM, action *Action) (uint64, error) {
	funcID, ok := vm.GetFunctionIndex(action.Field)
	if !ok {
		return 0, fmt.Errorf("function not found %s", action.Field)
	}
	args := make([]uint64, 0)
	for _, arg := range action.Args {
		val, err := strconv.ParseUint(arg.Value, 10, 64)
		if err != nil {
			panic(err)
		}
		args = append(args, val)
	}
	// t.Logf("Triggering %s with args at line %d", cmd.Action.Field, cmd.Line)
	return vm.Invoke(funcID, args...)
}

func TestWasmSuite(t *testing.T) {
	tests := []string{
		"i32", "i64", "f32", "f64",
		"f32_cmp", "f32_bitwise", "f64_cmp", "f64_bitwise",
		"br", "br_if", "br_table",
		"call", "call_indirect",
		"global", "local_get", "local_set", "local_tee",
		"memory", "memory_grow", "memory_size", "memory_redundancy", "memory_trap",
		"binary", "binary-leb128", "block",
		"address",
		"comments",
		"return", "select", "loop", "if",
		"custom", "endianness",
		"fac", "float_literals", "float_memory",
		"forward", "func",
		"inline-module", "int_exprs", "int_literals", "labels",
		"left-to-right", "load", "nop", "stack", "store", "switch", "token",
		"traps", "type", "unreachable", "unreached-invalid", "unwind",
		"utf8-custom-section-id", "utf8-import-field", "utf8-import-module", "utf8-invalid-encoding",
		"skip-stack-guard-page", "float_exprs", "float_misc", "align",
		"start", "func_ptrs",
		"const", "table", "break-drop",
		"conversions", "names",

		// "exports", // empty module removed
		// "linking",
		// "elem", "data", //wasm parsing failed
		// "imports", // missing imports from spec
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
		var vm, trapVM *VM
		for _, cmd := range suite.Commands {
			// t.Logf("Running test %s %d", name, cmd.Line)
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
				vm, err = NewVM(data, &FreeGasPolicy{}, &Gas{}, &TestResolver{})
				if err != nil {
					t.Error(err)
				}
				trapVM, err = NewVM(data, &FreeGasPolicy{}, &Gas{}, &TestResolver{})
				if err != nil {
					t.Error(err)
				}
			case "assert_return", "action", "assert_return_canonical_nan", "assert_return_arithmetic_nan":
				switch cmd.Action.Type {
				case "invoke":
					ret, err := invokeWithAction(vm, &cmd.Action)
					if err != nil {
						panic(err)
					}
					if len(cmd.Expected) != 0 {
						var exp uint64
						if cmd.Expected[0].Value == "nan:canonical" {
							if cmd.Expected[0].Type == "f32" {
								exp = 0x7fc00000
							} else if cmd.Expected[0].Type == "f64" {
								exp = 0x7ff8000000000000
							}
						} else if cmd.Expected[0].Value == "nan:arithmetic" {
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
					entry, ok := vm.Module.ExportSec.ExportMap[cmd.Action.Field]
					if !ok {
						panic("Global export not found")
					}
					ret := vm.globals[entry.Desc.Idx]
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
			case "assert_trap":
				if ret, err := invokeWithAction(trapVM, &cmd.Action); err != nil {
					if cmd.Text == "undefined element" {
						cmd.Text = "out of bounds table access"
					}
					if err.Error() != cmd.Text {
						t.Errorf("Test %s Line %d: Expect trap text to be %s, got %s", name, cmd.Line, cmd.Text, err)
					}
				} else {
					t.Errorf("Test %s Line %d: Expect trap text to be %s, returned %d instead", name, cmd.Line, cmd.Text, ret)
				}
			case "assert_invalid", "assert_malformed", "assert_uninstantiable", "assert_unlinkable", "assert_exhaustion":
				// t.Logf("Skipping %s", cmd.Type)
			default:
				t.Errorf("unknown command %s", cmd.Type)
			}
		}
	}
}
