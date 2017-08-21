package helpers

import (
	"bytes"
	"fmt"

	utilities "github.com/artificial-universe-maker/go-utilities"
	"github.com/artificial-universe-maker/go-utilities/models"

	"encoding/binary"
)

type Result struct {
	Value string
	Error error
}

// Evaluates the byte slice statement
// Returns an exec id
func evaluateStatement(state models.AumMutableRuntimeState, stmt []byte) (key string, eval bool) {
	key = ""
	eval = false

	return
}

// LogicLazyEval is used during AUM project user request runtime.
// When a request is made in-game, it's routed to the appropriate dialog
// The dialog has logical blocks attached therein,
// which yield Redis Keys for respective ActionBundle binaries
func LogicLazyEval(stateComms chan models.AumMutableRuntimeState, compiled []byte) <-chan Result {

	ch := make(chan Result)
	go func() {
		defer close(ch)

		reader := bytes.NewReader(compiled)
		r := utilities.ByteReader{
			Reader:   reader,
			Position: 0,
		}

		// Reading the "AlwaysExec" key
		// First get the length of the string
		barr, err := r.ReadNBytes(2)
		if err != nil {
			ch <- Result{Error: fmt.Errorf("Error reading AlwaysExec length: %s", err.Error())}
			return
		}
		strlen := binary.LittleEndian.Uint16(barr)

		// Read the Redis key for the AlwaysExec Action Bundle
		execkey, err := r.ReadNBytes(uint64(strlen))
		if err != nil {
			ch <- Result{Error: fmt.Errorf("Error reading AlwaysExec key: %s", err.Error())}
			return
		}

		// Dispatch
		ch <- Result{Value: string(execkey)}

		if r.Finished() {
			return
		}

		// Get the number of conditional statement blocks
		numStatements, err := r.ReadByte()
		if err != nil {
			ch <- Result{Error: err}
			return
		}

		awaitNewState := true
		var state models.AumMutableRuntimeState

		for i := 0; i < int(numStatements); i++ {
			barr, err := r.ReadNBytes(8)
			if err != nil {
				ch <- Result{Error: fmt.Errorf("Error reqading logical statement: %s", err.Error())}
				return
			}
			stmtlen := binary.LittleEndian.Uint64(barr)

			// evaluateStatement does not run in parallel
			if awaitNewState {
				state = <-stateComms
				awaitNewState = false
			}
			key, eval := evaluateStatement(state, compiled[r.Position:r.Position+stmtlen])
			if eval {
				// If the statement evaluated to true
				// Send the key for the ActionBundle back for processing
				ch <- Result{Value: key}
				// The ActionBundle will mutate the state
				// Therefore we must wait for a new one to pass
				// to the next evaluateStatement function
				awaitNewState = true
			}
		}

	}()

	return ch
}
