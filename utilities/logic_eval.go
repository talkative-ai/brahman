package utilities

import (
	"bytes"

	"encoding/binary"

	"github.com/artificial-universe-maker/brahman/models"
)

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

func LogicLazyEval(compiled []byte) ([]uint64, error) {

	ids := []uint64{}

	r := bytes.NewReader(compiled)

	hasAlways, err := r.ReadByte()
	if err != nil {
		return nil, err
	}

	if hasAlways == 1 {
		barr, err := readNBytes(r, 8)
		if err != nil {
			return nil, err
		}
		execID := binary.LittleEndian.Uint64(barr)
		ids = append(ids, execID)
	}

	conditionalLength, err := r.ReadByte()
	if err != nil {
		return nil, err
	}

	for i := 0; i < int(conditionalLength); i++ {
		b, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		expectedStatements := models.StatementInt(b)
		if expectedStatements&models.StatementIF > 0 {
			b, err = r.ReadByte()
			if err != nil {
				return nil, err
			}
		}
		if expectedStatements&models.StatementELIF > 0 {
			b, err = r.ReadByte()
			if err != nil {
				return nil, err
			}
			elifCount := uint8(b)
			var c uint8
			for c = 0; c < elifCount; c++ {
				b, err = r.ReadByte()
				if err != nil {
					return nil, err
				}
			}
		}
		if expectedStatements&models.StatementELSE > 0 {
			b, err = r.ReadByte()
			if err != nil {
				return nil, err
			}
		}
	}

	return ids, nil
}
