(module
  (type $t0 (func (param f64) (result i32)))
  (type $t1 (func (result i32)))
  (func $i32.trunc_f64_u (type $t0) (param $p0 f64) (result i32)
    local.get $p0
    i32.trunc_f64_u)
  (func $main (type $t1) (result i32)
    f64.const 0x1.ffffffffccccdp+31 (;=4.29497e+09;)
    call $i32.trunc_f64_u)
  (table $T0 1 1 funcref)
  (memory $memory 2)
  (global $g0 (mut i32) (i32.const 66560))
  (global $__heap_base i32 (i32.const 66560))
  (global $__data_end i32 (i32.const 1024))
  (export "main" (func $main))
  (export "i32.trunc_f64_u" (func $i32.trunc_f64_u))
  (export "memory" (memory 0))
  (export "__heap_base" (global 1))
  (export "__data_end" (global 2)))
