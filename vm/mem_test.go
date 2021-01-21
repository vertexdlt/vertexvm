package vm

import (
	"io"
	"reflect"
	"testing"
)

func TestMemSize(t *testing.T) {
	vm := GetTestVM("i32", &FreeGasPolicy{}, 0)
	if len(vm.memory) != vm.MemSize() {
		t.Errorf("Expect MemSize to be %d, got %d", len(vm.memory), vm.MemSize())
	}
}

func TestMemGrow(t *testing.T) {
	vm := GetTestVM("memory_grow", &SimpleGasPolicy{}, 1024*3+3)
	fnIndex, ok := vm.GetFunctionIndex("grow")
	if !ok {
		panic("Cannot get export fn index")
	}
	_, err := vm.Invoke(fnIndex)
	if err != nil {
		t.Errorf("Expect execution to go through, got %v", err)
	}
}

func TestMemGrowOutOfGas(t *testing.T) {
	vm := GetTestVM("memory_grow", &SimpleGasPolicy{}, 1024*2+3)
	fnIndex, ok := vm.GetFunctionIndex("grow")
	if !ok {
		panic("Cannot get export fn index")
	}
	_, err := vm.Invoke(fnIndex)
	if err != ErrOutOfGas {
		t.Errorf("Expect execution to be out of gas, got %v", err)
	}
}

func TestMemInitOutOfGas(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			err := r.(error)
			if err != ErrOutOfGas {
				t.Errorf("Expect execution to be out of gas, got %v", err)
			}
		}
	}()
	GetTestVM("memory_grow", &SimpleGasPolicy{}, 2047)
}

func TestMemRead(t *testing.T) {
	vm := GetTestVM("i32", &FreeGasPolicy{}, 0)
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
	vm := GetTestVM("i32", &FreeGasPolicy{}, 0)
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
