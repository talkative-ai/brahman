package intentHandlers

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/talkative-ai/go.uuid"

	actions "github.com/talkative-ai/actions-on-google-golang/model"
	"github.com/talkative-ai/core/common"
	"github.com/talkative-ai/core/db"
	"github.com/talkative-ai/core/models"
	"github.com/talkative-ai/core/redis"
)

// RandomStringCollection is a collection of potential strings categorized under a string key
type RandomStringCollection map[string][]string

// IntentResponses provide a variety of responses to generic requests
// TODO: Consider storing these as a database entry or external file so there's no deploy required
// with every single update?
var IntentResponses = RandomStringCollection{
	"unknown": []string{
		"I'm not sure I understand.",
		"Sorry, I don't think I get that.",
		"That doesn't make sense to me, sorry.",
	},
	"hint actions after list.apps": []string{
		"Or would you like to hear some genres?",
		"There's a lot of genres too.",
	},
	"wrap new title": []string{
		"Recently, an adventure named \"%s\" was published.",
		"There's this one called \"%s\" that's fresh off the press.",
	},
	"introduce": []string{
		"This is Talkative speaking. I hope you're having a good day.",
		"Talkative here, very nice to see you.",
		"Hello, you're speaking to Talkative. I hope you're having a great day.",
	},
	"instructions": []string{
		"You can say \"list apps\" to hear a list of user-generated content. Otherwise, try asking 'What is Talkative?'",
	},
	"talkative info": []string{
		`Talkative is a platform to create, publish, and play apps such as interactive stories.
		Talkative is free to use and generate content for. Learn more at our website, www.talkative.ai!
		To hear a list of apps, try saying "list apps"`,
	},
}

// ErrIntentNoMatch occurs when an intent handler does not match the current context
// For example, a user saying "cancel" for no reason
var ErrIntentNoMatch = fmt.Errorf("talkative:no_match")

// IntentHandler is a function signature for handling api.ai requests
type IntentHandler func(*actions.ApiAiRequest, *models.AIRequest) (contextOut *[]actions.ApiAiContext, err error)

// List maps ApiAi intents to functions
var List = map[string]IntentHandler{
	"Default Welcome Intent": Welcome,
	"app.initialize":         InitializeApp,
	"app.demo":               DemoApp,
	"info":                   Info,
	"list":                   ListApps,
	"app.stop":               AppStopHandler,
	"app.restart":            AppRestartHandler,
	"confirm":                ConfirmHandler,
	"cancel":                 CancelHandler,
	"help":                   HelpHandler,
	"repeat":                 RepeatHandler,
}

// Welcome IntentHandler provides an introduction to Talkative
func Welcome(input *actions.ApiAiRequest, message *models.AIRequest) (*[]actions.ApiAiContext, error) {
	if message.State.ProjectID != uuid.Nil {
		return nil, ErrIntentNoMatch
	}
	message.OutputSSML = message.OutputSSML.
		Text(common.ChooseString(IntentResponses["introduce"])).
		Text(common.ChooseString(IntentResponses["instructions"]))
	return nil, nil
}

// Info IntentHandler provides additional information on Talkative
func Info(input *actions.ApiAiRequest, message *models.AIRequest) (*[]actions.ApiAiContext, error) {
	if message.State.ProjectID != uuid.Nil {
		return nil, ErrIntentNoMatch
	}
	message.OutputSSML = message.OutputSSML.Text(common.ChooseString(IntentResponses["talkative info"]))
	return nil, nil
}

// ListApps IntentHandler provides additional information on Talkative
func ListApps(input *actions.ApiAiRequest, message *models.AIRequest) (*[]actions.ApiAiContext, error) {
	if message.State.ProjectID != uuid.Nil {
		return nil, ErrIntentNoMatch
	}

	var items []models.Project
	_, err := db.DBMap.Select(&items, `
		SELECT DISTINCT ON (pp."ProjectID")
			p."Title"
		FROM published_workbench_projects pp
		JOIN workbench_projects p
		ON p."ID" = pp."ProjectID"
		ORDER BY pp."ProjectID", pp."CreatedAt" DESC
		LIMIT 5
	`)
	if err != nil {
		return nil, err
	}

	message.OutputSSML.Text(`Some available apps to play are: `)
	for i := 0; i < len(items); i++ {
		if i > 0 {
			message.OutputSSML.Text(", another app is ")
		}
		message.OutputSSML.Text(fmt.Sprintf("'%v'", items[i].Title))
	}
	message.OutputSSML.Text(`. To play an app, say "Let's play" and then the name of the app.`)
	return nil, nil
}

// Unknown IntentHandler handles all unknown intents
func Unknown(input *actions.ApiAiRequest, message *models.AIRequest) (*[]actions.ApiAiContext, error) {
	contextOut := []actions.ApiAiContext{}
	for _, ctx := range input.Result.Contexts {
		if ctx.Name != "previous_output" {
			continue
		}
		contextOut = []actions.ApiAiContext{
			actions.ApiAiContext{
				Name: "previous_output",
				Parameters: map[string]string{
					"DisplayText": ctx.Parameters["DisplayText"],
					"Speech":      ctx.Parameters["Speech"],
				},
				Lifespan: 1,
			},
		}
	}
	message.OutputSSML = message.OutputSSML.Text(common.ChooseString(IntentResponses["unknown"])).
		Text(" Try saying 'help' if you're unsure what to do.")
	return &contextOut, nil
}

// DemoApp enables users to demo their own app before publishing it
func DemoApp(input *actions.ApiAiRequest, message *models.AIRequest) (*[]actions.ApiAiContext, error) {
	project := models.Project{}
	err := db.DBMap.SelectOne(&project, `
		SELECT "ID"
		FROM workbench_projects
		WHERE LOWER("Title")=LOWER($1)
	`, input.Result.Parameters["app"])
	if err != nil && err == sql.ErrNoRows {
		message.OutputSSML = message.OutputSSML.Text("Cannot find that app.")
		return nil, err
	} else if err != nil {
		message.OutputSSML = message.OutputSSML.Text("Sorry, there was a problem.")
		return nil, err
	}
	projectID := project.ID
	message.OutputSSML = message.OutputSSML.Text("Okay, found it.")
	message.State.ProjectID = projectID
	message.State.SessionID = uuid.NewV4()
	message.State.PubID = fmt.Sprintf("demo:%v", projectID.String())
	message.State.Demo = true
	var setup models.RAResetApp
	setup.Execute(message)
	return nil, nil
}

// InitializeApp IntentHandler will begin a specified app if it exists
func InitializeApp(input *actions.ApiAiRequest, message *models.AIRequest) (*[]actions.ApiAiContext, error) {
	if message.State.ProjectID != uuid.Nil {
		return nil, ErrIntentNoMatch
	}
	projectID := uuid.FromStringOrNil(redis.Instance.HGet(models.KeynavGlobalMetaProjects(), strings.ToUpper(input.Result.Parameters["app"])).Val())
	if projectID == uuid.Nil {
		message.OutputSSML = message.OutputSSML.Text("Sorry, that one doesn't exist yet! Try saying 'help' if you're unsure what to do next.")
		return nil, nil
	}
	message.State.ProjectID = projectID
	message.State.PubID = projectID.String()
	message.State.SessionID = uuid.NewV4()
	message.OutputSSML = message.OutputSSML.Text(fmt.Sprintf("Okay, starting %v. Have fun!", input.Result.Parameters["app"]))
	var setup models.RAResetApp
	setup.Execute(message)
	return nil, nil
}

func AppStopHandler(input *actions.ApiAiRequest, runtimeState *models.AIRequest) (*[]actions.ApiAiContext, error) {
	runtimeState.OutputSSML.Text(`
		Okay, stopping the app now. You're back to the main menu.
		If you're not sure what to do, say "help"`)
	return nil, nil
}

func AppRestartHandler(input *actions.ApiAiRequest, runtimeState *models.AIRequest) (*[]actions.ApiAiContext, error) {
	requestedRestart := false
	for _, ctx := range input.Result.Contexts {
		if ctx.Name != "requested_restart" {
			continue
		}
		requestedRestart = true
	}
	if requestedRestart {
		runtimeState.OutputSSML.Text(`Okay, restarting now...`)
		var setup models.RAResetApp
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

func ConfirmHandler(input *actions.ApiAiRequest, runtimeState *models.AIRequest) (*[]actions.ApiAiContext, error) {
	for _, ctx := range input.Result.Contexts {
		if ctx.Name == "requested_restart" {
			runtimeState.OutputSSML.Text(`Okay, restarting now...`)
			var setup models.RAResetApp
			setup.Execute(runtimeState)
			return nil, nil
		}
	}
	return nil, ErrIntentNoMatch
}

func CancelHandler(input *actions.ApiAiRequest, runtimeState *models.AIRequest) (*[]actions.ApiAiContext, error) {
	for _, ctx := range input.Result.Contexts {
		if ctx.Name == "requested_restart" {
			runtimeState.OutputSSML.Text(`Okay, you've cancelled restarting. Try saying 'help' if you're unsure what to do next.`)
			return nil, nil
		}
	}
	return nil, ErrIntentNoMatch
}

func HelpHandler(input *actions.ApiAiRequest, runtimeState *models.AIRequest) (*[]actions.ApiAiContext, error) {
	if runtimeState.State.ProjectID == uuid.Nil {
		runtimeState.OutputSSML.Text(`
			You can say "list apps" to hear what's available,
			"help" to hear this help menu,
			and "quit" to leave.`)
	} else {
		runtimeState.OutputSSML.Text(`
			You can say "repeat" to hear the last thing over again,
			"stop app" to leave the current app,
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

func RepeatHandler(input *actions.ApiAiRequest, runtimeState *models.AIRequest) (*[]actions.ApiAiContext, error) {
	for _, ctx := range input.Result.Contexts {
		if ctx.Name != "previous_output" {
			continue
		}
		runtimeState.OutputSSML.Text(ctx.Parameters["Speech"])
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
	return Welcome(input, runtimeState)
}
