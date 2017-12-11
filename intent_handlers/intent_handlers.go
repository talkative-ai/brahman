package intentHandlers

import (
	"fmt"
	"log"
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

// IntentHandler is a function signature for handling api.ai requests
type IntentHandler func(*actions.ApiAiRequest, *models.AumMutableRuntimeState)

// List maps ApiAi intents to functions
var List = map[string]IntentHandler{
	"Default Welcome Intent": Welcome,
	"app.initialize":         InitializeGame,
	"info":                   Info,
	"list":                   ListApps,
}

// Welcome IntentHandler provides an introduction to AUM
func Welcome(q *actions.ApiAiRequest, message *models.AumMutableRuntimeState) {
	message.OutputSSML = message.OutputSSML.Text(common.ChooseString(IntentResponses["introduce"]))
	message.OutputSSML = message.OutputSSML.Text(common.ChooseString(IntentResponses["instructions"]))
}

// Info IntentHandler provides additional information on AUM
func Info(q *actions.ApiAiRequest, message *models.AumMutableRuntimeState) {
	message.OutputSSML = message.OutputSSML.Text(common.ChooseString(IntentResponses["aum info"]))
}

// ListApps IntentHandler provides additional information on AUM
func ListApps(q *actions.ApiAiRequest, message *models.AumMutableRuntimeState) {
	message.OutputSSML = message.OutputSSML.Text(common.ChooseString(IntentResponses["aum info"]))
}

// Unknown IntentHandler handles all unknown intents
func Unknown(q *actions.ApiAiRequest, message *models.AumMutableRuntimeState) {
	message.OutputSSML = message.OutputSSML.Text(common.ChooseString(IntentResponses["unknown"]))
}

// InitializeGame IntentHandler will begin a specified game if it exists
func InitializeGame(q *actions.ApiAiRequest, message *models.AumMutableRuntimeState) {
	redis, err := providers.ConnectRedis()
	defer redis.Close()
	if err != nil {
		log.Println("Error", err)
		return
	}
	projectID := uuid.FromStringOrNil(redis.HGet(models.KeynavGlobalMetaProjects(), strings.ToUpper(q.Result.Parameters["game"])).Val())
	if projectID == uuid.Nil {
		message.OutputSSML = message.OutputSSML.Text("Sorry, that one doesn't exist yet!")
		return
	}
	message.State.PubID = projectID
	message.State.ZoneActors = map[uuid.UUID][]string{}
	message.State.ZoneInitialized = map[uuid.UUID]bool{}
	for _, zoneID := range redis.SMembers(
		fmt.Sprintf("%v:%v", models.KeynavProjectMetadataStatic(projectID.String()), "all_zones")).Val() {
		zUUID := uuid.FromStringOrNil(zoneID)
		message.State.ZoneActors[zUUID] =
			redis.SMembers(models.KeynavCompiledActorsWithinZone(projectID.String(), zoneID)).Val()
		message.State.ZoneInitialized[zUUID] = false
	}
	message.OutputSSML = message.OutputSSML.Text(fmt.Sprintf("Okay, starting %v. Have fun!", q.Result.Parameters["game"]))
	zoneID := redis.HGet(models.KeynavProjectMetadataStatic(projectID.String()), "start_zone_id").Val()
	setZone := models.ARASetZone(uuid.FromStringOrNil(zoneID))
	setZone.Execute(message)

}
