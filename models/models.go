package models

type OperatorInt int8

const (
	OpIntEQ OperatorInt = 1 << iota
	OpIntLT
	OpIntGT
	OpIntLE
	OpIntGE
	OpIntNE
)
