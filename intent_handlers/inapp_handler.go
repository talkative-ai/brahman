package intentHandlers

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/talkative-ai/snips-nlu-types"

	goredis "github.com/go-redis/redis"
	"github.com/talkative-ai/core/db"
	"github.com/talkative-ai/core/models"
	"github.com/talkative-ai/core/redis"
	uuid "github.com/talkative-ai/go.uuid"
)

func MatchIntent(key, query string) (*snips.Result, error) {

	var result snips.Result

	data := url.Values{}
	data.Set("query", query)
	// Note the context here is set to App, rather than Talkative
	// because this isn't a conversation with Talkative,
	// it's a conversation with the app
	data.Set("context", key)

	rq, err := http.NewRequest("POST", fmt.Sprintf("http://kalidasa:8080/v1/parse"), strings.NewReader(data.Encode()))
	if err != nil {
		fmt.Println("Error in TrainData", err)
		// TODO: Handle errors
	}
	rq.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	rq.Header.Add("Content-Length", strconv.Itoa(len(data.Encode())))

	client := http.Client{}
	resp, err := client.Do(rq)

	rawResponse, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
		// TODO: Handle errors
	}

	json.Unmarshal(rawResponse, &result)

	return &result, nil
}

func InAppHandler(rawInput string, message *models.AIRequest) error {
	projectID := message.State.ProjectID
	pubID := message.State.PubID

	fmt.Printf("InApp Message: %+v\nSTATE = %+v\n", rawInput, message.State)

	var dialogID string
	eventIDChan := make(chan uuid.UUID)
	if !message.State.Demo {
		go func() {
			var newID uuid.UUID
			err := db.Instance.QueryRow(`INSERT INTO event_user_action ("UserID", "ProjectID", "RawInput") VALUES ($1, $2, $3) RETURNING "ID"`, uuid.Nil, projectID, rawInput).Scan(&newID)
			if err != nil {
				// TODO: Log this error somewhere
			}
			eventIDChan <- newID
		}()
	}

	if message.State.CurrentDialog != nil {
		// Attempt to fetch a dialog relative to the current dialog.
		// Otherwise known as dialog node children
		// This is where conversational context works
		currentDialogKey := *message.State.CurrentDialog
		split := strings.Split(currentDialogKey, ":")
		currentDialogID := split[len(split)-1]
		input := models.DialogInput(rawInput)
		result, err := MatchIntent(models.KeynavCompiledDialogNode(pubID, currentDialogID), input.Prepared())
		if err != nil {
			return err
		}
		// TODO: Generalize probability threshold
		if result.Intent.Probability > 0.8 {
			dialogID = result.Intent.Name
		}
		fmt.Printf("Result in current dialog, %+v\n", result)
	} else {
		// If there is no current dialog, then we scan all "root dialogs"
		// for the actors within the Zone
		// This is where conversations begin
		for _, actorID := range message.State.ZoneActors[message.State.Zone] {
			input := models.DialogInput(rawInput)
			result, err := MatchIntent(models.KeynavCompiledDialogRootWithinActor(pubID, actorID), input.Prepared())
			fmt.Printf("Result in root dialogs attempt: %+v\n", result)
			if err != nil {
				return err
			}
			// TODO: Generalize probability threshold
			if result.Intent.Probability > 0.8 {
				dialogID = result.Intent.Name
				fmt.Printf("Result in root dialogs, %+v\n", result)
				break
			}
		}
	}

	// There were no dialogs at all with the given input
	// So we check to see if there's a "catch-all" unknown dialog handler
	if dialogID == "" {
		fmt.Println("There were no dialogs.")
		if message.State.CurrentDialog != nil {
			currentDialogKey := *message.State.CurrentDialog
			split := strings.Split(currentDialogKey, ":")
			currentDialogID := split[len(split)-1]
			v := redis.Instance.Get(models.KeynavCompiledDialogNodeUnknown(pubID, currentDialogID))
			if v.Err() == nil || v.Err() == goredis.Nil {
				dialogID = v.Val()
			} else {
				v.Err()
			}
		} else {
			for _, actorID := range message.State.ZoneActors[message.State.Zone] {
				v := redis.Instance.Get(models.KeynavCompiledDialogRootUnknownWithinActor(pubID, actorID))
				if v.Err() == nil || v.Err() == goredis.Nil {
					dialogID = v.Val()
					break
				} else {
					v.Err()
				}
			}
		}
	}

	// Still nothing, so abort with a default unknown response
	// TODO: We should allow modifying the default unknown response.
	// This probably won't happen in the future but eventually will need to consider.
	// e.g. attach default unknown response to the zone? actor? etc.
	if dialogID == "" {
		return ErrIntentNoMatch
	}

	dialogBinary, err := redis.Instance.Get(dialogID).Bytes()
	if err != nil {
		return err
	}
	stateComms := make(chan models.AIRequest, 1)
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
			return err
		}
		bundleBinary, err := redis.Instance.Get(res.Value).Bytes()
		if err != nil {
			return err
		}
		err = models.ActionBundleEval(message, bundleBinary)
		if err != nil {
			return err
		}
		stateComms <- *message
		stateChange = true
	}
	// TODO: Reenable
	stateChange = false
	if stateChange && !message.State.Demo {
		newID := <-eventIDChan
		stateObject, _ := message.State.Value()
		go db.Instance.QueryRow(`INSERT INTO event_state_change ("EventUserActionID", "StateObject") VALUES ($1, $2)`, newID, stateObject)
	}

	return nil
}
