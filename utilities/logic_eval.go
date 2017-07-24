package utilities

import (
	"bytes"

	"encoding/binary"
)

type execIndex struct {
	execID uint64
	Index  int
}

func readNBytes(r *bytes.Reader, n int) ([]byte, error) {
	bslice := []byte{}

	for i := 0; i < n; i++ {
		b, err := r.ReadByte()
		if err != nil {
			return []byte{}, err
		}
		bslice = append(bslice, b)
	}

	return bslice, nil
}

// Evaluates the byte slice statement
// Sends back an exec id through the channel
func evaluateStatement(stmt []byte, idx int, c chan execIndex) {
}

func LogicLazyEval(compiled []byte) ([]uint64, error) {

	ids := []uint64{}
	var bytesRead uint64

	r := bytes.NewReader(compiled)

	hasAlways, err := r.ReadByte()
	bytesRead++
	if err != nil {
		return nil, err
	}

	if hasAlways == 1 {
		barr, err := readNBytes(r, 8)
		bytesRead += 8
		if err != nil {
			return nil, err
		}
		execID := binary.LittleEndian.Uint64(barr)
		ids = append(ids, execID)
	}

	numStatements, err := r.ReadByte()
	bytesRead++
	if err != nil {
		return nil, err
	}

	c := make(chan execIndex)

	for i := 0; i < int(numStatements); i++ {
		barr, err := readNBytes(r, 8)
		if err != nil {
			return nil, err
		}
		stmtlen := binary.LittleEndian.Uint64(barr)
		bytesRead += 8
		go evaluateStatement(compiled[bytesRead:stmtlen], i, c)
		bytesRead += stmtlen
	}

	reg := 0
	orderedIDs := make([]*uint64, numStatements)
	for val := range c {
		reg++
		if reg >= int(numStatements) {
			close(c)
		}
		orderedIDs[val.Index] = &val.execID
	}

	for i := 0; i < int(numStatements); i++ {
		if orderedIDs[i] != nil {
			ids = append(ids, *orderedIDs[i])
		}
	}

	return ids, nil
}
