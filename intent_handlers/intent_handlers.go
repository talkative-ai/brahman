package intentHandlers

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/talkative-ai/go.uuid"
	snips "github.com/talkative-ai/snips-nlu-types"

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
		"Talkative here, very nice to hear from you.",
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
type IntentHandler func(*snips.Result, *models.AIRequest) error

// List maps ApiAi intents to functions
var List = map[string]IntentHandler{
	"talkative.welcome":        Welcome,
	"talkative.info":           TalkativeInfo,
	"talkative.list":           TalkativeListApps,
	"talkative.help":           TalkativeHelp,
	"talkative.app.initialize": TalkativeInitialize,
	"talkative.app.demo":       TalkativeAppDemo,
	"app.stop":                 AppStop,
	"app.restart":              AppRestart,
	"app.help":                 AppHelp,
	"confirm":                  ConfirmHandler,
	"cancel":                   CancelHandler,
}

// Welcome IntentHandler provides an introduction to Talkative
func Welcome(input *snips.Result, message *models.AIRequest) error {
	message.OutputSSML = message.OutputSSML.
		Text(common.ChooseString(IntentResponses["introduce"])).
		Text(common.ChooseString(IntentResponses["instructions"]))
	return nil
}

// Info IntentHandler provides additional information on Talkative
func TalkativeInfo(input *snips.Result, message *models.AIRequest) error {
	message.OutputSSML = message.OutputSSML.Text(common.ChooseString(IntentResponses["talkative info"]))
	return nil
}

// ListApps IntentHandler provides additional information on Talkative
func TalkativeListApps(input *snips.Result, message *models.AIRequest) error {

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
		return err
	}

	message.OutputSSML.Text(`Some available apps to play are: `)
	for i := 0; i < len(items); i++ {
		if i > 0 {
			message.OutputSSML.Text(", another app is ")
		}
		message.OutputSSML.Text(fmt.Sprintf("'%v'", items[i].Title))
	}
	message.OutputSSML.Text(`. To play an app, say "Let's play" and then the name of the app.`)
	return nil
}

// Unknown IntentHandler handles all unknown intents
func Unknown(input *snips.Result, message *models.AIRequest) error {
	message.OutputSSML = message.OutputSSML.Text(common.ChooseString(IntentResponses["unknown"])).
		Text(" Try saying 'help' if you're unsure what to do.")
	return nil
}

// DemoApp enables users to demo their own app before publishing it
func TalkativeAppDemo(input *snips.Result, message *models.AIRequest) error {
	project := models.Project{}
	err := db.DBMap.SelectOne(&project, `
		SELECT "ID"
		FROM workbench_projects
		WHERE LOWER("Title")=LOWER($1)
	`, input.SlotsMappedByName()["appName"].RawValue)
	if err != nil && err == sql.ErrNoRows {
		message.OutputSSML = message.OutputSSML.Text("Cannot find that app.")
		return nil
	} else if err != nil {
		message.OutputSSML = message.OutputSSML.Text("Sorry, there was a problem.")
		return err
	}
	projectID := project.ID
	message.OutputSSML = message.OutputSSML.Text("Okay, found it.")
	message.State.ProjectID = projectID
	message.State.SessionID = uuid.NewV4()
	message.State.PubID = fmt.Sprintf("demo:%v", projectID.String())
	message.State.Demo = true
	var setup models.RAResetApp
	setup.Execute(message)
	return nil
}

// InitializeApp IntentHandler will begin a specified app if it exists
func TalkativeInitialize(input *snips.Result, message *models.AIRequest) error {

	var appName string

	if slot, ok := input.SlotsMappedByName()["appName"]; ok {
		appName = slot.RawValue
	}

	projectID := uuid.FromStringOrNil(redis.Instance.HGet(models.KeynavGlobalMetaProjects(), strings.ToUpper(appName)).Val())
	if projectID == uuid.Nil {
		message.OutputSSML = message.OutputSSML.Text("Sorry, that one doesn't exist yet! Try saying 'help' if you're unsure what to do next.")
		return nil
	}
	message.State.ProjectID = projectID
	message.State.PubID = projectID.String()
	message.State.SessionID = uuid.NewV4()
	message.OutputSSML = message.OutputSSML.Text(fmt.Sprintf("Okay, starting %v. Have fun!", appName))
	var setup models.RAResetApp
	setup.Execute(message)
	return nil
}

func AppStop(input *snips.Result, runtimeState *models.AIRequest) error {
	runtimeState.OutputSSML.Text(`
		Okay, stopping the app now. You're back to the main menu.
		If you're not sure what to do, say "help"`)
	runtimeState.State = models.MutableAIRequestState{}
	return nil
}

func AppRestart(input *snips.Result, runtimeState *models.AIRequest) error {
	if runtimeState.State.RestartRequested {
		runtimeState.State.RestartRequested = false
		runtimeState.OutputSSML.Text(`Okay, restarting now...`)
		var setup models.RAResetApp
		setup.Execute(runtimeState)
		return nil
	}
	runtimeState.OutputSSML.Text(`All of your progress will be lost forever. If you're sure, say "I'm sure". Otherwise, say "cancel".`)
	runtimeState.State.RestartRequested = true
	// TODO: Manage restart requested here
	return nil
}

func ConfirmHandler(input *snips.Result, runtimeState *models.AIRequest) error {
	if !runtimeState.State.RestartRequested {
		return ErrIntentNoMatch
	}

	runtimeState.OutputSSML.Text(`Okay, restarting now...`)
	var setup models.RAResetApp
	setup.Execute(runtimeState)
	return nil
}

func CancelHandler(input *snips.Result, runtimeState *models.AIRequest) error {
	if !runtimeState.State.RestartRequested {
		return ErrIntentNoMatch
	}

	runtimeState.OutputSSML.Text(`Okay, you've cancelled restarting. Try saying 'help' if you're unsure what to do next.`)
	return nil
}

func TalkativeHelp(input *snips.Result, runtimeState *models.AIRequest) error {
	runtimeState.OutputSSML.Text(`
		You can say "list apps" to hear what's available,
		"help" to hear this help menu,
		and "quit" to leave.`)
	return nil
}

func AppHelp(input *snips.Result, runtimeState *models.AIRequest) error {
	runtimeState.OutputSSML.Text(`
		You can say "repeat that" to repeat the last thing from the app,
		"stop app" to leave the current app,
		"restart app" to start from the beginning erasing all of your progress,
		and "help" to hear this help menu.`)
	return nil
}
