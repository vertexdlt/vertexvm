package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"github.com/perlin-network/life/exec"
	"github.com/vertexdlt/vertexvm/vm"
	"golang.org/x/crypto/sha3"
)

var storageMap = make(map[[32]byte][]byte)

// Debug
var totalMatched int64

func readAt(vm *vm.VM, ptr, size int) []byte {
	data := vm.GetMemory()[ptr : ptr+size]
	return data
}

func printBytes(vm *vm.VM, args ...uint64) uint64 {
	ptr := int(uint32(args[0]))
	size := int(uint32(args[1]))
	key := readAt(vm, ptr, size)
	// fmt.Println("readAt", ptr, key)
	// fmt.Println("printBytes", string(key))
	matched := strings.Split(string(key), " ")[2]
	ret, _ := strconv.ParseInt(matched, 10, 0)
	totalMatched += ret
	fmt.Println(string(key))
	// fmt.Println("Parsed to", ret, ret*(ret+1)/2, totalMatched, string(key))
	return 0
}

func setStorage(vm *vm.VM, args ...uint64) uint64 {
	keyPtr := int(uint32(args[0]))
	keySize := int(uint32(args[1]))
	valuePtr := int(uint32(args[2]))
	valueSize := int(uint32(args[3]))
	// fmt.Println("keyPtr", keyPtr, "valuePtr", valuePtr)
	key := readAt(vm, keyPtr, keySize)
	value := readAt(vm, valuePtr, valueSize)
	storageMap[sha3.Sum256(key)] = value
	return 0
}

func getStorage(vm *vm.VM, args ...uint64) uint64 {
	keyPtr := int(uint32(args[0]))
	keySize := int(uint32(args[1]))
	key := readAt(vm, keyPtr, keySize)
	valuePtr := int(uint32(args[2]))
	// fmt.Println("keyPtr", keyPtr, "key", string(key))
	value := storageMap[sha3.Sum256(key)]
	if len(value) > 0 {
		copy(vm.GetMemory()[valuePtr:], value)
	}
	return uint64(valuePtr)
}

func getValueSize(vm *vm.VM, args ...uint64) uint64 {
	keyPtr := int(uint32(args[0]))
	keySize := int(uint32(args[1]))
	key := readAt(vm, keyPtr, keySize)
	value := storageMap[sha3.Sum256(key)]
	return uint64(len(value))
}

func syscall(vm *vm.VM, args ...uint64) uint64 {
	fmt.Println("syscall", args)
	idx := int(uint32(args[0]))
	if idx == 45 {
		return 0
	} else if idx == 192 {
		requested := int(uint32(args[2]))
		fmt.Println("mmap2", requested)
		cur := len(vm.GetMemory())
		n := (requested-1)/exec.DefaultPageSize + 1
		fmt.Println("requesting pages:", n, exec.DefaultPageSize, len(vm.GetMemory()))
		vm.ExtendMemory(n)
		fmt.Println("first heap mem", cur, len(vm.GetMemory()))
		return uint64(cur)
	} else {
		fmt.Printf("syscall idx %d: NYI\n", idx)
	}
	return 0
}

type Resolver struct{}

func (r *Resolver) GetFunction(module, name string) vm.HostFunction {
	switch module {
	case "env":
		switch name {
		case "print_bytes":
			return printBytes
		case "set_storage":
			return setStorage
		case "get_storage":
			return getStorage
		case "get_value_size":
			return getValueSize
		case "__syscall0", "__syscall1", "__syscall2", "__syscall3", "__syscall4", "__syscall5", "__syscall6":
			return syscall
		default:
			panic(fmt.Errorf("unknown import resolved: %s", name))
		}
	}
	return nil
}

func main() {
	fileName := os.Args[1]
	fmt.Println("fileName", fileName)
	input, err := ioutil.ReadFile(fileName)
	if err != nil {
		panic(err)
	}
	vm, err := vm.NewVM(input, &Resolver{})
	if err != nil {
		panic(err)
	}

	entryID, ok := vm.GetFunctionIndex("add_order_state")
	if !ok {
		panic("entry function not found")
	}

	ret := vm.Invoke(entryID, 100000, 1, 0)

	for i := 0; i < 123; i++ {
		if i%1000 == 0 {
			fmt.Println("Sell", i+1)
		}
		ret = vm.Invoke(entryID, 8700, uint64(i+1), 0)
	}

	for i := 0; i < 123; i++ {
		if i%1000 == 0 {
			fmt.Println("Buy", i+1)
		}
		ret = vm.Invoke(entryID, 8700, uint64(i+1), 1)
	}
	fmt.Println("TotalMatched", totalMatched)
	fmt.Printf("return value = %d\n", ret)
}
