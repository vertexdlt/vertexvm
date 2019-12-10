(module
  (type $t0 (func (param i32)))
  (type $t1 (func (param i32) (result i32)))
  (type $t2 (func))
  (import "wasi_unstable" "proc_exit" (func $__wasi_proc_exit (type $t0)))
  (func $calc (type $t1) (param $p0 i32) (result i32)
    i32.const 1
    call $exit
    unreachable)
  (func $_Exit (type $t0) (param $p0 i32)
    get_local $p0
    call $__wasi_proc_exit
    unreachable)
  (func $dummy (type $t2))
  (func $exit (type $t0) (param $p0 i32)
    call $dummy
    call $dummy
    get_local $p0
    call $_Exit
    unreachable)
  (table $T0 1 1 anyfunc)
  (memory $memory 2)
  (global $g0 (mut i32) (i32.const 66560))
  (export "memory" (memory 0))
  (export "calc" (func $calc)))
