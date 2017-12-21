package intentHandlers

import (
	actions "github.com/artificial-universe-maker/actions-on-google-golang/model"
	"github.com/artificial-universe-maker/core/models"
)

// List maps ApiAi intents to functions
var SpecialHandlers = map[string]IntentHandler{
	"app.stop":    AppStopHandler,
	"app.restart": AppRestartHandler,
	"confirm":     ConfirmHandler,
	"cancel":      CancelHandler,
	"help":        HelpHandler,
	"repeat":      RepeatHandler,
}

func AppStopHandler(input *actions.ApiAiRequest, runtimeState *models.AumMutableRuntimeState) (*[]actions.ApiAiContext, error) {
	return nil, nil
}

func AppRestartHandler(input *actions.ApiAiRequest, runtimeState *models.AumMutableRuntimeState) (*[]actions.ApiAiContext, error) {
	requestedRestart := false
	for _, ctx := range input.Result.Contexts {
		if ctx.Name != "requested_restart" {
			continue
		}
		requestedRestart = true
	}
	if requestedRestart {
		runtimeState.OutputSSML.Text(`Okay, restarting now...`)
		var setup models.ARAResetApp
		setup.Execute(runtimeState)
		return nil, nil
	}
	runtimeState.OutputSSML.Text(`Are you sure you want to restart the app? All of your progress will be lost forever.`)
	contextOut := []actions.ApiAiContext{
		actions.ApiAiContext{
			Name:       "requested_restart",
			Parameters: map[string]string{},
			Lifespan:   1,
		},
	}
	return &contextOut, nil
}

func ConfirmHandler(input *actions.ApiAiRequest, runtimeState *models.AumMutableRuntimeState) (*[]actions.ApiAiContext, error) {
	for _, ctx := range input.Result.Contexts {
		if ctx.Name == "requested_restart" {
			runtimeState.OutputSSML.Text(`Okay, restarting now...`)
			var setup models.ARAResetApp
			// TODO:
			// This may create a null pointer exception. Keep an eye out!
			setup.Execute(runtimeState)
			return nil, nil
		}
	}
	return nil, ErrIntentNoMatch
}

func CancelHandler(input *actions.ApiAiRequest, runtimeState *models.AumMutableRuntimeState) (*[]actions.ApiAiContext, error) {
	for _, ctx := range input.Result.Contexts {
		if ctx.Name == "requested_restart" {
			runtimeState.OutputSSML.Text(`Okay, you've cancelled restarting.`)
			return nil, nil
		}
	}
	return nil, ErrIntentNoMatch
}

func HelpHandler(input *actions.ApiAiRequest, runtimeState *models.AumMutableRuntimeState) (*[]actions.ApiAiContext, error) {
	runtimeState.OutputSSML.Text(`
		You can say "repeat" to hear the last thing over again,
		"stop app" to leave the current app,
		"restart app" to start from the beginning erasing all of your progress,
		and "help" to hear this help menu.`)
	for _, ctx := range input.Result.Contexts {
		if ctx.Name != "previous_output" {
			continue
		}
		contextOut := []actions.ApiAiContext{
			actions.ApiAiContext{
				Name: "previous_output",
				Parameters: map[string]string{
					"DisplayText": ctx.Parameters["DisplayText"],
					"Speech":      ctx.Parameters["Speech"],
				},
				Lifespan: 1,
			},
		}
		return &contextOut, nil
	}
	return nil, nil
}

func RepeatHandler(input *actions.ApiAiRequest, runtimeState *models.AumMutableRuntimeState) (*[]actions.ApiAiContext, error) {
	for _, ctx := range input.Result.Contexts {
		if ctx.Name != "previous_output" {
			continue
		}
		runtimeState.OutputSSML.Text(ctx.Parameters["DisplayText"])
		outputContext := []actions.ApiAiContext{
			actions.ApiAiContext{
				Name: "previous_output",
				Parameters: map[string]string{
					"DisplayText": ctx.Parameters["DisplayText"],
					"Speech":      ctx.Parameters["Speech"],
				},
				Lifespan: 1,
			},
		}
		return &outputContext, nil
	}
	return nil, ErrIntentNoMatch
}
