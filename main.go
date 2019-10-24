package main

import (
	"fmt"
	"io/ioutil"

	"github.com/vertexdlt/vertexvm/vm"
)

func main() {
	data, err := ioutil.ReadFile("vm/test_data/block.wasm")
	if err != nil {
		panic(err)
	}
	machine, err := vm.NewVM(data, nil)
	if err != nil {
		panic(err)
	}
	fnIndex, ok := machine.GetFunctionIndex("main")
	if !ok {
		panic("cannot get fn export")
	}
	fmt.Println(machine.Module.FunctionIndexSpace)
	for i, fn := range machine.Module.FunctionIndexSpace {
		fmt.Println("func", i, fn.Body.Exprs)
	}
	fmt.Println(machine.Invoke(fnIndex))
}
