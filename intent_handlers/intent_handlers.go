package intentHandlers

import (
	"fmt"
	"strings"

	"github.com/artificial-universe-maker/go.uuid"

	actions "github.com/artificial-universe-maker/actions-on-google-golang/model"
	"github.com/artificial-universe-maker/core/common"
	"github.com/artificial-universe-maker/core/models"
	"github.com/artificial-universe-maker/core/providers"
)

// RandomStringCollection is a collection of potential strings categorized under a string key
type RandomStringCollection map[string][]string

// IntentResponses provide a variety of responses to generic requests
var IntentResponses = RandomStringCollection{
	"unknown": []string{
		"I'm not sure I understand.",
		"Sorry, I don't think I get that.",
		"That doesn't make sense to me, sorry.",
	},
	"hint actions after list.games": []string{
		"Or would you like to hear some genres?",
		"There's a lot of genres too.",
	},
	"wrap new title": []string{
		"Recently, an adventure named \"%s\" was published.",
		"There's this one called \"%s\" that's fresh off the press.",
	},
	"introduce": []string{
		"This is AUM speaking. I hope you're having a good day.",
		"AUM here, very nice to see you.",
		"Hello, you're speaking to AUM. I hope you're having a great day.",
	},
	"instructions": []string{
		"You can say \"List\" to hear a list of playable AUM applications. Otherwise, try asking 'What is AUM?'",
	},
	"aum info": []string{
		`AUM is a platform to create, publish, and play voice apps like interactive stories.
		AUM is free to use. Read more at our website, www.aum.ai!
		To hear a list of apps you can say "List"`,
	},
}

// ErrIntentNoMatch occurs when an intent handler does not match the current context
// For example, a user saying "cancel" for no reason
var ErrIntentNoMatch error = fmt.Errorf("aum:no_match")

// IntentHandler is a function signature for handling api.ai requests
type IntentHandler func(*actions.ApiAiRequest, *models.AumMutableRuntimeState) (contextOut *[]actions.ApiAiContext, err error)

// List maps ApiAi intents to functions
var List = map[string]IntentHandler{
	"Default Welcome Intent": Welcome,
	"app.initialize":         InitializeGame,
	"info":                   Info,
	"list":                   ListApps,
	"app.stop":               AppStopHandler,
	"app.restart":            AppRestartHandler,
	"confirm":                ConfirmHandler,
	"cancel":                 CancelHandler,
	"help":                   HelpHandler,
	"repeat":                 RepeatHandler,
}

// Welcome IntentHandler provides an introduction to AUM
func Welcome(q *actions.ApiAiRequest, message *models.AumMutableRuntimeState) (*[]actions.ApiAiContext, error) {
	message.OutputSSML = message.OutputSSML.Text(common.ChooseString(IntentResponses["introduce"]))
	message.OutputSSML = message.OutputSSML.Text(common.ChooseString(IntentResponses["instructions"]))
	return nil, nil
}

// Info IntentHandler provides additional information on AUM
func Info(q *actions.ApiAiRequest, message *models.AumMutableRuntimeState) (*[]actions.ApiAiContext, error) {
	message.OutputSSML = message.OutputSSML.Text(common.ChooseString(IntentResponses["aum info"]))
	return nil, nil
}

// ListApps IntentHandler provides additional information on AUM
func ListApps(q *actions.ApiAiRequest, message *models.AumMutableRuntimeState) (*[]actions.ApiAiContext, error) {
	message.OutputSSML = message.OutputSSML.Text(common.ChooseString(IntentResponses["aum info"]))
	return nil, nil
}

// Unknown IntentHandler handles all unknown intents
func Unknown(q *actions.ApiAiRequest, message *models.AumMutableRuntimeState) (*[]actions.ApiAiContext, error) {
	message.OutputSSML = message.OutputSSML.Text(common.ChooseString(IntentResponses["unknown"]))
	return nil, nil
}

// InitializeGame IntentHandler will begin a specified game if it exists
func InitializeGame(q *actions.ApiAiRequest, message *models.AumMutableRuntimeState) (*[]actions.ApiAiContext, error) {
	redis, err := providers.ConnectRedis()
	defer redis.Close()
	if err != nil {
		return nil, err
	}
	projectID := uuid.FromStringOrNil(redis.HGet(models.KeynavGlobalMetaProjects(), strings.ToUpper(q.Result.Parameters["game"])).Val())
	if projectID == uuid.Nil {
		message.OutputSSML = message.OutputSSML.Text("Sorry, that one doesn't exist yet!")
		return nil, nil
	}
	message.State.PubID = projectID
	message.OutputSSML = message.OutputSSML.Text(fmt.Sprintf("Okay, starting %v. Have fun!", q.Result.Parameters["game"]))
	var setup models.ARAResetApp
	setup.Execute(message)
	return nil, nil
}

func AppStopHandler(input *actions.ApiAiRequest, runtimeState *models.AumMutableRuntimeState) (*[]actions.ApiAiContext, error) {
	runtimeState.OutputSSML.Text(`
		Okay, stopping the app now. You're back to the main menu.
		If you're not sure what to do, say "help"`)
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
	if runtimeState.State.PubID == uuid.Nil {
		runtimeState.OutputSSML.Text(`
			You can say "list apps" to hear the apps in the multiverse,
			"help" to hear this help menu,
			and "quit" to leave.`)
	} else {
		runtimeState.OutputSSML.Text(`
			You can say "repeat" to hear the last thing over again,
			"stop app" to save your progress and leave the current app,
			"restart app" to start from the beginning erasing all of your progress,
			and "help" to hear this help menu.`)
	}
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
