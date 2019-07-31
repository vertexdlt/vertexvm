package main

import (
	"fmt"
	"io/ioutil"

	"github.com/vertexdlt/vm/vm"
)

func main() {
	data, err := ioutil.ReadFile("vm/test_data/i32.wasm")
	if err != nil {
		panic(err)
	}
	machine, err := vm.NewVM(data)
	if err != nil {
		panic(err)
	}
	fmt.Println(machine.Invoke(1))
}
