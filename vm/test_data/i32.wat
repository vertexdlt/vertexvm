(module
  (type $t0 (func))
  (type $t1 (func (result i32)))
  (func $__wasm_call_ctors (type $t0))
  (func $calc (export "calc") (type $t1) (result i32)
    i32.const -512
    i32.const 9
    i32.sub
    i32.const 32
    i32.div_s
    i32.const 10
    i32.rem_s
    i32.const 7
    i32.xor
    i32.const 91
    i32.and
    i32.const 3
    i32.shl
    i32.const 2
    i32.rotl
    i32.const 2
    i32.rotr
    i32.const -712
    i32.div_s
    return)
  (table $T0 1 1 anyfunc)
  (memory $memory (export "memory") 2)
  (global $g0 (mut i32) (i32.const 66560))
  (global $__heap_base (export "__heap_base") i32 (i32.const 66560))
  (global $__data_end (export "__data_end") i32 (i32.const 1024)))
