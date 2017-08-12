package helpers

import (
	"bytes"

	"encoding/binary"
)

type Result struct {
	Indexed ResultIndex
	Error   error
}

type ResultIndex struct {
	Index             int
	KeyToActionBundle string
}

type ByteReader struct {
	Reader   *bytes.Reader
	Position int64
}

func (br *ByteReader) ReadNBytes(n int64) ([]byte, error) {
	bslice := []byte{}

	for i := int64(0); i < n; i++ {
		b, err := br.Reader.ReadByte()
		if err != nil {
			return []byte{}, err
		}
		bslice = append(bslice, b)
	}

	br.Position += n

	return bslice, nil
}

func (br *ByteReader) ReadByte() (byte, error) {
	br.Position++
	return br.Reader.ReadByte()
}

// Evaluates the byte slice statement
// Sends back an exec id through the channel
func evaluateStatement(stmt []byte, idx int, c chan ResultIndex) {
}

// LogicLazyEval is used during AUM project user request runtime.
// When a request is made in-game, it's routed to the appropriate dialog
// The dialog has logical blocks attached therein,
// which yield Redis Keys for respective ActionBundle binaries
func LogicLazyEval(compiled []byte) <-chan Result {

	ch := make(chan Result)
	go func() {
		defer func() {
			close(ch)
		}()
		var bytesRead uint64

		reader := bytes.NewReader(compiled)
		r := ByteReader{
			Reader:   reader,
			Position: 0,
		}

		// Reading the "AlwaysExec" string
		// First get the length of the string
		barr, err := r.ReadNBytes(2)
		if err != nil {
			ch <- Result{Error: err}
			return
		}
		strlen := binary.LittleEndian.Uint16(barr)

		// Read the Redis key for the AlwaysExec Action Bundle
		execkey, err := r.ReadNBytes(int64(strlen))
		if err != nil {
			ch <- Result{Error: err}
			return
		}

		// Dispatch
		ch <- Result{Indexed: ResultIndex{0, string(execkey)}}

		// Get the number of conditional statement blocks
		numStatements, err := r.ReadByte()
		bytesRead++
		if err != nil {
			ch <- Result{Error: err}
			return
		}

		// Async fetch remaining valid Redis keys
		c := make(chan ResultIndex)
		for i := 0; i < int(numStatements); i++ {
			barr, err := r.ReadNBytes(8)
			if err != nil {
				ch <- Result{Error: err}
				return
			}
			stmtlen := binary.LittleEndian.Uint64(barr)
			bytesRead += 8

			// i+1 because ExecAlways index == 0
			go evaluateStatement(compiled[bytesRead:stmtlen], i+1, c)
			bytesRead += stmtlen
		}

		i := 1
		for val := range c {
			// evaluateStatement will pass along the Redis key
			// Order preserved by the Result.Indexed
			ch <- Result{Indexed: val}
			if i >= int(numStatements) {
				close(c)
			}
		}
	}()

	return ch
}
