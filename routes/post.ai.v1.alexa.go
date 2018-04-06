package routes

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/talkative-ai/brahman/intent_handlers"
	ssml "github.com/talkative-ai/go-ssml"
	snips "github.com/talkative-ai/snips-nlu-types"

	"github.com/gorilla/mux"
	"github.com/talkative-ai/go-alexa/skillserver"
	uuid "github.com/talkative-ai/go.uuid"

	"github.com/talkative-ai/core/models"
	"github.com/talkative-ai/core/myerrors"
	"github.com/talkative-ai/core/redis"
	"github.com/talkative-ai/core/router"
)

// GetProjects router.Route
// Path: "/user/register",
// Method: "GET",
// Accepts models.TokenValidate
// Responds with status of success or failure
var PostAlexa = &router.Route{
	Path:   "/ai/v1/alexa/{projectID:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}}",
	Method: "POST",
}

func PostAlexaHandler(w http.ResponseWriter, r *http.Request) {

	urlparams := mux.Vars(r)
	echoReq := r.Context().Value("echoRequest").(*skillserver.EchoRequest)
	aiRequest := models.AIRequest{
		State:      models.MutableAIRequestState{},
		OutputSSML: ssml.NewBuilder(),
	}
	projectID, err := uuid.FromString(urlparams["projectID"])
	if err != nil {
		// If the ID isn't a valid UUID, return a bad request error
		myerrors.Respond(w, &myerrors.MySimpleError{
			Code:    http.StatusBadRequest,
			Message: "bad_id",
			Req:     r,
		})
		return
	}
	aiRequest.State.ProjectID = projectID
	aiRequest.State.PubID = projectID.String()
	isRepeat := false
	isExit := false

	if echoReq.Session.New {
		var setup models.RAResetApp
		setup.Execute(&aiRequest)
	} else {
		stateString, err := redis.Instance.Get(models.KeynavContextConversation(echoReq.Session.SessionID)).Result()
		if err != nil {
			myerrors.Respond(w, &myerrors.MySimpleError{
				Code:    http.StatusBadRequest,
				Message: "bad_state",
				Req:     r,
				Log:     err.Error(),
			})
			return
		}
		json.Unmarshal([]byte(stateString), &aiRequest.State)

		parsedInput := snips.Result{}

		var rawInput string
		if rawSlot, ok := echoReq.Request.Intent.Slots["Raw"]; ok {
			if rawSlot.Resolutions.ResolutionsPerAuthority[0].Values != nil {
				rawInput = (*rawSlot.Resolutions.ResolutionsPerAuthority[0].Values)[0].Value.Name
			} else {
				rawInput = rawSlot.Value
			}
		} else {
			parsedInput.Intent.Name = echoReq.Request.Intent.Name
			parsedInput.Intent.Probability = 1
		}

		if parsedInput.Intent.Name == "" {
			data := url.Values{}
			data.Set("query", models.DialogInput(rawInput).Prepared())
			// Note the context here is set to App, rather than Talkative
			// because this isn't a conversation with Talkative,
			// it's a conversation with the app
			data.Set("context", models.KeynavStaticIntentsApp())

			rq, err := http.NewRequest("POST", fmt.Sprintf("http://kalidasa:8080/v1/parse"), strings.NewReader(data.Encode()))
			if err != nil {
				fmt.Println("Error in TrainData", err)
				// TODO: Handle errors
			}
			r.Header.Add("Content-Type", "application/x-www-form-urlencoded")
			r.Header.Add("Content-Length", strconv.Itoa(len(data.Encode())))

			client := http.Client{}
			resp, err := client.Do(rq)

			rawResponse, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				fmt.Println("Error in TrainData", err)
				return
				// TODO: Handle errors
			}
			json.Unmarshal(rawResponse, &parsedInput)
		}

		intentHandled := false
		if parsedInput.Intent.Name == "repeat" {
			intentHandled = true
			isRepeat = true
		} else if parsedInput.Intent.Name == "app.stop" {
			intentHandled = true
			isExit = true
		} else if handler, ok := intentHandlers.List[parsedInput.Intent.Name]; ok {
			err = handler(&parsedInput, &aiRequest)
			if err == nil {
				intentHandled = true
			}
			if err != nil && err != intentHandlers.ErrIntentNoMatch {
				myerrors.Respond(w, &myerrors.MySimpleError{
					Code:    http.StatusBadRequest,
					Message: "unknown_error",
					Req:     r,
					Log:     err.Error(),
				})
				return
			}
		}

		if !intentHandled {
			err = intentHandlers.InAppHandler(rawInput, &aiRequest)
			if err == intentHandlers.ErrIntentNoMatch {
				intentHandlers.Unknown(nil, &aiRequest)
			} else if err != nil {
				myerrors.Respond(w, &myerrors.MySimpleError{
					Code:    http.StatusBadRequest,
					Message: "unknown_error",
					Req:     r,
					Log:     err.Error(),
				})
				return
			}
		}
	}

	echoResp := skillserver.NewEchoResponse()
	if isRepeat {
		json.Unmarshal([]byte(aiRequest.State.PreviousResponse), echoResp)
	} else {
		echoResp = echoResp.OutputSpeechSSML(aiRequest.OutputSSML.String())
		previousResponseByte, err := json.Marshal(echoResp)
		if err != nil {
			myerrors.Respond(w, &myerrors.MySimpleError{
				Code:    http.StatusBadRequest,
				Message: "unknown_error",
				Req:     r,
				Log:     err.Error(),
			})
			return
		}
		aiRequest.State.PreviousResponse = string(previousResponseByte)
	}

	stateBytes, err := json.Marshal(aiRequest.State)
	if err != nil {
		myerrors.Respond(w, &myerrors.MySimpleError{
			Code:    http.StatusBadRequest,
			Message: "bad_state_marshal",
			Req:     r,
			Log:     err.Error(),
		})
		return
	}
	redis.Instance.Set(models.KeynavContextConversation(echoReq.Session.SessionID), stateBytes, time.Hour*720)

	if !isExit {
		echoResp = echoResp.EndSession(false)
	} else {
		echoResp = echoResp.EndSession(true)
	}

	json, _ := echoResp.String()
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	w.Write(json)
}
