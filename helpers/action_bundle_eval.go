package helpers

import (
	"bytes"
	"encoding/binary"

	utilities "github.com/artificial-universe-maker/go-utilities"
	"github.com/artificial-universe-maker/go-utilities/models"
)

func ActionBundleEval(state *models.AumMutableRuntimeState, bundle []byte) error {
	var r utilities.ByteReader
	r.Reader = bytes.NewReader(bundle)

	for !r.Finished() {
		barr, err := r.ReadNBytes(8)
		if err != nil {
			return err
		}
		actionID := binary.LittleEndian.Uint64(barr)
		action := models.GetActionFromID(models.AumActionID(actionID))

		barr, err = r.ReadNBytes(4)
		if err != nil {
			return err
		}

		actionLength := binary.LittleEndian.Uint32(barr)
		actionBytes, err := r.ReadNBytes(uint64(actionLength))
		if err != nil {
			return err
		}
		action.CreateFrom(actionBytes)
		action.Execute(state)
	}

	return nil
}
