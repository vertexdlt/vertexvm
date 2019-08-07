(module
  (type $t0 (func))
  (type $t1 (func (param i32) (result i32)))
  (func $__wasm_call_ctors (type $t0))
  (func $calc (export "calc") (type $t1) (param $p0 i32) (result i32)
    (local $l0 i32)
    get_local $p0
    if
        i32.const 5
        set_local $l0
    else
        i32.const 7
        set_local $l0
    end
    get_local $l0
  )
  (table $T0 1 1 anyfunc)
  (memory $memory (export "memory") 2)
  (global $g0 (mut i32) (i32.const 66560))
  (global $__heap_base (export "__heap_base") i32 (i32.const 66560))
  (global $__data_end (export "__data_end") i32 (i32.const 1024)))
