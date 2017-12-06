package intentHandlers

import (
	"log"
	"strings"

	actions "github.com/artificial-universe-maker/actions-on-google-golang/model"
	"github.com/artificial-universe-maker/core/db"
	"github.com/artificial-universe-maker/core/models"
	"github.com/artificial-universe-maker/core/providers"
	uuid "github.com/artificial-universe-maker/go.uuid"
)

func IngameHandler(q *actions.ApiAiRequest, message *models.AumMutableRuntimeState) {
	redis, err := providers.ConnectRedis()
	if err != nil {
		log.Fatal("Error connecting to redis", err)
		return
	}
	projectID := message.State.PubID

	err = db.InitializeDB()
	if err != nil {
		log.Fatal("Error parsing projectID", err)
		return
	}
	var dialogID string
	eventIDChan := make(chan uuid.UUID)
	go func() {
		var newID uuid.UUID
		err = db.Instance.QueryRow(`INSERT INTO event_user_action ("UserID", "PubID", "RawInput") VALUES ($1, $2, $3) RETURNING "ID"`, uuid.Nil, projectID, q.Result.ResolvedQuery).Scan(&newID)
		if err != nil {
			// TODO: Log this error somewhere
			return
		}
		eventIDChan <- newID
	}()
	if message.State.CurrentDialog != nil {
		currentDialogKey := *message.State.CurrentDialog
		split := strings.Split(currentDialogKey, ":")
		currentDialogID := split[len(split)-1]
		for _, actorID := range message.State.ZoneActors[message.State.Zone] {
			v := redis.HGet(models.KeynavCompiledDialogNodeWithinActor(projectID.String(), actorID, currentDialogID), strings.ToUpper(q.Result.ResolvedQuery))
			if v.Err() == nil {
				dialogID = v.Val()
				break
			}
		}
	} else {
		for _, actorID := range message.State.ZoneActors[message.State.Zone] {
			v := redis.HGet(models.KeynavCompiledDialogRootWithinActor(projectID.String(), actorID), strings.ToUpper(q.Result.ResolvedQuery))
			if v.Err() == nil {
				dialogID = v.Val()
				break
			}
		}
	}

	if dialogID == "" {
		Unknown(q, message)
		return
	}

	dialogBinary, err := redis.Get(dialogID).Bytes()
	if err != nil {
		log.Fatal("Error fetching logic binary", dialogID, err)
		return
	}
	stateComms := make(chan models.AumMutableRuntimeState, 1)
	defer close(stateComms)
	dialogEnd := dialogBinary[0] == 0
	dialogBinary = dialogBinary[1:]
	if dialogEnd {
		message.State.CurrentDialog = nil
	} else {
		message.State.CurrentDialog = &dialogID
	}
	stateChange := false
	result := models.LogicLazyEval(stateComms, dialogBinary)
	for res := range result {
		if res.Error != nil {
			log.Fatal("Error with logic evaluation", res.Error)
			return
		}
		bundleBinary, err := redis.Get(res.Value).Bytes()
		if err != nil {
			log.Fatal("Error fetching action bundle binary", err)
			return
		}
		err = models.ActionBundleEval(message, bundleBinary)
		if err != nil {
			log.Fatal("Error processing action bundle binary", err)
			return
		}
		stateComms <- *message
		stateChange = true
	}
	if stateChange {
		newID := <-eventIDChan
		stateObject, _ := message.State.Value()
		go db.Instance.QueryRow(`INSERT INTO event_state_change ("EventUserActionID", "StateObject") VALUES ($1, $2)`, newID, stateObject)
	}
}
