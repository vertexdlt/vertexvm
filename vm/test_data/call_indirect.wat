(module
  (type $t0 (func (param i32) (result i32)))
  (type $t1 (func))
  (type $t2 (func (result i32)))
  (func $__wasm_call_ctors (type $t1))
  (func $addNumber (type $t0) (param $p0 i32) (result i32)
    i32.const 1
    get_local $p0
    i32.add)
  (func $myFunction (type $t0) (param $p0 i32) (result i32)
    i32.const 5
    get_local $p0
    call_indirect (type $t0)
    i32.const 10
    i32.add)
  (func $calc (export "calc") (type $t2) (result i32)
    i32.const 1
    call $myFunction)
  (table $T0 2 2 anyfunc)
  (memory $memory (export "memory") 2)
  (global $g0 (mut i32) (i32.const 66560))
  (global $__heap_base (export "__heap_base") i32 (i32.const 66560))
  (global $__data_end (export "__data_end") i32 (i32.const 1024))
  (elem (i32.const 1) $addNumber))
