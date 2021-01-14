package number

type TrapCode byte

const (
	NoTrap      TrapCode = 0
	NanTrap     TrapCode = 1
	ConvertTrap TrapCode = 2
)
