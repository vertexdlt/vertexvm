package number

import "math"

func Min(t Type) uint64 {
	switch t {
	case I32:
		i := math.MinInt32
		return uint64(i)
	case I64:
		i := math.MinInt64
		return uint64(i)
	case U32, U64:
		return 0
	}
	panic("invalid type")
}
func Max(t Type) uint64 {
	switch t {
	case I32:
		return uint64(math.MaxInt32)
	case I64:
		return uint64(math.MaxInt64)
	case U32:
		return uint64(math.MaxUint32)
	case U64:
		return math.MaxUint64
	}
	panic("invalid type")
}
