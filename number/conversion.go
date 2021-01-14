package number

import (
	"math"
)

// CanTruncate checks if a float (from) can be converted to an int (to)
func CanTruncate(from Type, to Type, value interface{}) bool {
	if from == F32 && to == I32 {
		if v, ok := value.(float32); ok {
			return math.MinInt32 <= v && v < math.MaxInt32+1
		}
		panic("Check value must be float32")
	}
	if from == F64 && to == I32 {
		if v, ok := value.(float64); ok {
			return math.MinInt32-1 < v && v < math.MaxInt32+1
		}
		panic("Check value must be float64")
	}
	if from == F32 && to == U32 {
		if v, ok := value.(float32); ok {
			return -1 < v && v < math.MaxUint32+1
		}
		panic("Check value must be float32")
	}
	if from == F64 && to == U32 {
		if v, ok := value.(float64); ok {
			return -1 < v && v < math.MaxUint32+1
		}
		panic("Check value must be float32")
	}
	if from == F32 && to == I64 {
		if v, ok := value.(float32); ok {
			return math.MinInt64 <= v && v < math.MaxInt64+1
		}
		panic("Check value must be float32")
	}
	if from == F64 && to == I64 {
		if v, ok := value.(float64); ok {
			return math.MinInt64 <= v && v < math.MaxInt64+1
		}
		panic("Check value must be float64")
	}
	if from == F32 && to == U64 {
		if v, ok := value.(float32); ok {
			return -1 < v && v < math.MaxUint64+1
		}
		panic("Check value must be float32")
	}
	if from == F64 && to == U64 {
		if v, ok := value.(float64); ok {
			return -1 < v && v < math.MaxUint64+1
		}
		panic("Check value must be float32")
	}
	panic("Invalid conversion types")
}

// FloatTruncate truncates a float represented by floatBits to an integer (signed or unsigned)
// when it cannot perform the operation it returns the corresponding trap codes
func FloatTruncate(from Type, to Type, floatBits uint64) (uint64, TrapCode) {
	var r uint64
	if from == F32 {
		f := math.Float32frombits(uint32(floatBits))
		if math.IsNaN(float64(f)) {
			return 0, NanTrap
		} else if !CanTruncate(from, to, f) {
			if math.Signbit(float64(f)) {
				return Min(to), ConvertTrap
			}
			return Max(to), ConvertTrap
		}
		switch to {
		case I32:
			r = uint64(int32(f))
		case I64:
			r = uint64(int64(f))
		case U32:
			r = uint64(uint32(f))
		case U64:
			r = uint64(f)
		default:
			panic("to must be an int")
		}
	} else if from == F64 {
		f := math.Float64frombits(floatBits)
		if math.IsNaN(f) {
			return 0, NanTrap
		} else if !CanTruncate(from, to, f) {
			if math.Signbit(f) {
				return Min(to), ConvertTrap
			}
			return Max(to), ConvertTrap
		}
		switch to {
		case I32:
			r = uint64(int32(f))
		case I64:
			r = uint64(int64(f))
		case U32:
			r = uint64(uint32(f))
		case U64:
			r = uint64(f)
		default:
			panic("to must be an int")
		}
	} else {
		panic("from must be a float")
	}
	return r, NoTrap
}
