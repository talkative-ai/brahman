package helpers

import (
	"fmt"
	"net/url"
	"testing"

	ssml "github.com/artificial-universe-maker/go-ssml"
	"github.com/artificial-universe-maker/go-utilities/models"
	"github.com/artificial-universe-maker/lakshmi/prepare"
)

func TestActionBundleEval(t *testing.T) {
	AAS := models.AumActionSet{}
	AAS.PlaySounds = make([]models.ARAPlaySound, 2)
	AAS.PlaySounds[0].SoundType = models.ARAPlaySoundTypeText
	AAS.PlaySounds[0].Value = "Hello world"
	AAS.PlaySounds[1].SoundType = models.ARAPlaySoundTypeAudio
	AAS.PlaySounds[1].Value, _ = url.Parse("https://upload.wikimedia.org/wikipedia/commons/b/bb/Test_ogg_mp3_48kbps.wav")

	runtimeState := models.AumMutableRuntimeState{
		State:      map[string]interface{}{},
		OutputSSML: ssml.NewBuilder(),
	}

	bundled := prepare.BundleActions(AAS)

	err := ActionBundleEval(&runtimeState, bundled)
	if err != nil {
		fmt.Println(err)
		t.Fail()
	}

	if runtimeState.OutputSSML.String() != `<speak>Hello world<audio src="https://upload.wikimedia.org/wikipedia/commons/b/bb/Test_ogg_mp3_48kbps.wav" /></speak>` {
		fmt.Println("Unexpected runtimeState Output SSML")
		t.Fail()
	}
}
