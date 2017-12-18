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
		// Attempt to fetch a dialog relative to the current dialog.
		// Otherwise known as dialog node children
		// This is where conversational context works
		currentDialogKey := *message.State.CurrentDialog
		split := strings.Split(currentDialogKey, ":")
		currentDialogID := split[len(split)-1]
		for _, actorID := range message.State.ZoneActors[message.State.Zone] {
			input := models.AumDialogInput(q.Result.ResolvedQuery)
			v := redis.HGet(models.KeynavCompiledDialogNodeWithinActor(projectID.String(), actorID, currentDialogID), input.Prepared())
			if v.Err() == nil {
				dialogID = v.Val()
				break
			}
		}
	} else {
		// If there is no current dialog, then we scan all "root dialogs"
		// for the actors within the Zone
		// This is where conversations begin
		for _, actorID := range message.State.ZoneActors[message.State.Zone] {
			input := models.AumDialogInput(q.Result.ResolvedQuery)
			v := redis.HGet(models.KeynavCompiledDialogRootWithinActor(projectID.String(), actorID), input.Prepared())
			if v.Err() == nil {
				dialogID = v.Val()
				break
			}
		}
	}

	// There were no dialogs at all with the given input
	// So we check to see if there's a "catch-all" unknown dialog handler
	if dialogID == "" {
		if message.State.CurrentDialog != nil {
			currentDialogKey := *message.State.CurrentDialog
			split := strings.Split(currentDialogKey, ":")
			currentDialogID := split[len(split)-1]
			for _, actorID := range message.State.ZoneActors[message.State.Zone] {
				v := redis.HGet(models.KeynavCompiledDialogNodeWithinActor(projectID.String(), actorID, currentDialogID), models.AumDialogSpecialInputUnknown)
				if v.Err() == nil {
					dialogID = v.Val()
					break
				}
			}
		} else {
			for _, actorID := range message.State.ZoneActors[message.State.Zone] {
				v := redis.HGet(models.KeynavCompiledDialogRootWithinActor(projectID.String(), actorID), models.AumDialogSpecialInputUnknown)
				if v.Err() == nil {
					dialogID = v.Val()
					break
				}
			}
		}
	}

	// Still nothing, so abort with a default unknown response
	// TODO: We should allow modifying the default unknown response.
	// This probably won't happen in the future but eventually will need to consider.
	// e.g. attach default unknown response to the zone? actor? etc.
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
	// TODO: Reenable
	stateChange = false
	if stateChange {
		newID := <-eventIDChan
		stateObject, _ := message.State.Value()
		go db.Instance.QueryRow(`INSERT INTO event_state_change ("EventUserActionID", "StateObject") VALUES ($1, $2)`, newID, stateObject)
	}
}
