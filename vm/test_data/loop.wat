(module
  (type $t0 (func))
  (type $t1 (func (param i32) (result i32)))
  (func $__wasm_call_ctors (type $t0))
  (func $calc (export "calc") (type $t1) (param $p0 i32) (result i32)
    (local $l0 i32) (local $l1 i32)
    i32.const 0
    local.set $l0
    i32.const 0
    local.set $l1
    block  
      loop  
        i32.const 1
        local.get $l0
        i32.add
        local.set $l0
        local.get $l0
        local.get $p0
        i32.eq
        br_if 1
        local.get $l0
        local.get $l1
        i32.add
        local.set $l1
        br 0
      end
    end
    local.get $l1)
  (func $isPrime (export "isPrime") (type $t1) (param $p0 i32) (result i32)
    (local $l0 i32) (local $l1 i32)
    i32.const 1
    set_local $l0
    block $B0
      block $B1
        get_local $p0
        i32.const 3
        i32.lt_u
        br_if $B1
        i32.const 2
        set_local $l1
        loop $L2
          get_local $p0
          get_local $l1
          i32.rem_u
          i32.eqz
          br_if $B0
          i32.const 1
          set_local $l0
          get_local $l1
          i32.const 1
          i32.add
          tee_local $l1
          get_local $p0
          i32.lt_u
          br_if $L2
        end
      end
      get_local $l0
      return
    end
    get_local $l1)
  (table $T0 1 1 anyfunc)
  (memory $memory (export "memory") 2)
  (global $g0 (mut i32) (i32.const 66560))
  (global $__heap_base (export "__heap_base") i32 (i32.const 66560))
  (global $__data_end (export "__data_end") i32 (i32.const 1024)))
