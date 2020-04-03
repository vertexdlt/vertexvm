(module
  (type $t0 (func (result i32)))
  (func $failed_access (export "failed_access") (type $t0) (result i32)
    i32.const 131069 
    i32.load
    return)
  (func $access (export "access") (type $t0) (result i32)
    i32.const 131068
    i32.load
    return)
  (table $T0 1 1 anyfunc)
  (memory $memory (export "memory") 2)
  (global $g0 (mut i32) (i32.const 66560))
  (global $__heap_base (export "__heap_base") i32 (i32.const 66560))
  (global $__data_end (export "__data_end") i32 (i32.const 1024)))
