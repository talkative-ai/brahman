package intentHandlers

import (
	"fmt"
	"log"
	"strconv"
	"strings"

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
		"This is your buddy AUM.",
		"This is AUM speaking.",
		"AUM here, very nice to see you.",
	},
}

// IntentHandler is a function signature for handling api.ai requests
type IntentHandler func(*actions.ApiAiRequest, *models.AumMutableRuntimeState)

// List maps ApiAi intents to functions
var List = map[string]IntentHandler{
	"input.welcome":   Welcome,
	"game.initialize": InitializeGame,
}

// Welcome IntentHandler provides an introduction to AUM
func Welcome(q *actions.ApiAiRequest, message *models.AumMutableRuntimeState) {
	message.OutputSSML = message.OutputSSML.Text(common.Choose(IntentResponses["introduce"]).(string))
}

// Unknown IntentHandler handles all unknown intents
func Unknown(q *actions.ApiAiRequest, message *models.AumMutableRuntimeState) {
	message.OutputSSML = message.OutputSSML.Text(common.Choose(IntentResponses["unknown"]).(string))
}

// InitializeGame IntentHandler will begin a specified game if it exists
func InitializeGame(q *actions.ApiAiRequest, message *models.AumMutableRuntimeState) {
	redis, err := providers.ConnectRedis()
	if err != nil {
		log.Println("Error", err)
		return
	}
	projectIDString := redis.HGet(models.KeynavGlobalMetaProjects(), strings.ToUpper(q.Result.Parameters["game"])).Val()
	projectID, err := strconv.ParseUint(projectIDString, 10, 64)
	if err != nil {
		log.Println("Error", err)
		return
	}
	message.State.PubID = projectIDString
	message.State.ZoneActors = map[string][]string{}
	for _, zidString := range redis.SMembers(
		fmt.Sprintf("%v:%v", models.KeynavProjectMetadataStatic(uint64(projectID)), "all_zones")).Val() {
		zid, err := strconv.ParseUint(zidString, 10, 64)
		if err != nil {
			log.Println("Error", err)
			return
		}
		message.State.ZoneActors[zidString] =
			redis.SMembers(models.KeynavCompiledActorsWithinZone(projectID, zid)).Val()
		message.State.ZoneInitialized[zidString] = false
	}
	message.OutputSSML = message.OutputSSML.Text(fmt.Sprintf("Okay, starting %v. Have fun!", q.Result.Parameters["game"]))

	zoneID := redis.HGet(models.KeynavProjectMetadataStatic(uint64(projectID)), "start_zone_id").Val()
	zoneIDInt, _ := strconv.ParseUint(zoneID, 10, 64)
	setZone := models.ARASetZone(zoneIDInt)
	setZone.Execute(message)

}
