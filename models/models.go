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

type StatementInt int8

const (
	StatementIF StatementInt = 1 << iota
	StatementELIF
	StatementELSE
)
