package routes

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/talkative-ai/core"

	"github.com/talkative-ai/aog"
	"github.com/talkative-ai/snips-nlu-types"

	"github.com/talkative-ai/brahman/intent_handlers"
	"github.com/talkative-ai/core/models"
	"github.com/talkative-ai/core/prehandle"
	"github.com/talkative-ai/core/router"
	ssml "github.com/talkative-ai/go-ssml"
	uuid "github.com/talkative-ai/go.uuid"
)

// GetProjects router.Route
// Path: "/user/register",
// Method: "GET",
// Accepts models.TokenValidate
// Responds with status of success or failure
var PostGoogle = &router.Route{
	Path:       "/ai/v1/google",
	Method:     "POST",
	Handler:    http.HandlerFunc(postGoogleHander),
	Prehandler: []prehandle.Prehandler{prehandle.SetJSON},
}

// AIRequestHandler handles requests that expect language parsing and an AI response
// Currently expects ApiAi requests
// This is the core functionality of Brahman, which routes to appropriate IntentHandlers
func postGoogleHander(w http.ResponseWriter, r *http.Request) {

	if os.Getenv("REDIS_ADDR") == "" {
		os.Setenv("REDIS_ADDR", "127.0.0.1:6379")
		os.Setenv("REDIS_PASSWORD", "")
	}

	w.Header().Add("content-type", "application/json")

	requestState := &models.AIRequest{
		State:      models.MutableAIRequestState{},
		OutputSSML: ssml.NewBuilder(),
	}

	parsedRequest := &aog.Request{}
	err := json.NewDecoder(r.Body).Decode(parsedRequest)
	if err != nil {
		log.Print("Error:", err)
		return
	}

	parsedInput := snips.Result{}

	stateMap, err := utilities.ParseJTWClaims(parsedRequest.Conversation.ConversationToken)
	if stateMap["data"] != nil {
		stateMap = stateMap["data"].(map[string]interface{})
	}

	var isInApp bool
	if stateMap["ProjectID"] != nil && stateMap["ProjectID"].(string) != uuid.Nil.String() {
		isInApp = true
	}

	if isInApp {
		// TOOD: Handle errors

		requestState.State = models.MutableAIRequestState{
			Demo:      stateMap["Demo"].(bool),
			SessionID: uuid.FromStringOrNil(stateMap["SessionID"].(string)),
			Zone:      uuid.FromStringOrNil(stateMap["Zone"].(string)),
			PubID:     stateMap["PubID"].(string),
			ProjectID: uuid.FromStringOrNil(stateMap["ProjectID"].(string)),
		}
		requestState.State.ZoneActors = map[uuid.UUID][]string{}
		if stateMap["ZoneActors"] != nil {
			for zone, actors := range stateMap["ZoneActors"].(map[string]interface{}) {
				requestState.State.ZoneActors[uuid.FromStringOrNil(zone)] = []string{}
				for _, actor := range actors.([]interface{}) {
					requestState.State.ZoneActors[uuid.FromStringOrNil(zone)] = append(requestState.State.ZoneActors[uuid.FromStringOrNil(zone)], actor.(string))
				}
			}
		}
		if (stateMap["CurrentDialog"]) == nil {
			requestState.State.CurrentDialog = nil
		} else {
			s := stateMap["CurrentDialog"].(string)
			requestState.State.CurrentDialog = &s
		}

		// TODO: Generalize this and create consistency between brahman/intent_handlers
		if stateMap["ZoneInitialized"] != nil {
			requestState.State.ZoneInitialized = map[uuid.UUID]bool{}
			for key, item := range stateMap["ZoneInitialized"].(map[string]interface{}) {
				requestState.State.ZoneInitialized[uuid.FromStringOrNil(key)] = item.(bool)
			}
		}

		data := url.Values{}
		data.Set("query", parsedRequest.Inputs[0].RawInputs[0].Query)
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

	} else if parsedRequest.Conversation.Type != "NEW" {
		data := url.Values{}
		data.Set("query", parsedRequest.Inputs[0].RawInputs[0].Query)
		// Note the context here is set to Talkative, rather than App
		data.Set("context", models.KeynavStaticIntentsTalkative())

		rq, err := http.NewRequest("POST", fmt.Sprintf("http://kalidasa:8080/v1/parse"), strings.NewReader(data.Encode()))
		if err != nil {
			fmt.Println("Error in TrainData", err)
			return
			// TODO: Handle errors
		}
		rq.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		rq.Header.Add("Content-Length", strconv.Itoa(len(data.Encode())))

		client := http.Client{}
		resp, err := client.Do(rq)

		rawResponse, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			fmt.Println("Error in TrainData", err)
			return
			// TODO: Handle errors
		}

		json.Unmarshal(rawResponse, &parsedInput)
	} else {
		parsedInput.Intent.Name = "talkative.welcome"
	}

	intentHandled := false
	if handler, ok := intentHandlers.List[parsedInput.Intent.Name]; ok {
		err = handler(&parsedInput, requestState)
		if err == nil {
			intentHandled = true
		}
		if err != nil && err != intentHandlers.ErrIntentNoMatch {
			fmt.Println("Error", err)
			return
		}
	}

	if isInApp && !intentHandled {
		err = intentHandlers.InappHandler(parsedRequest.Inputs[0].RawInputs[0].Query, requestState)
		if err == nil {
			intentHandled = true
		}
		if err != nil && err != intentHandlers.ErrIntentNoMatch {
			fmt.Println("Error", err)
			return
		}
	}

	if !intentHandled {
		err = intentHandlers.Unknown(&parsedInput, requestState)
		if err != nil {
			fmt.Println("Error", err)
			return
		}
	}

	// if input.Result.Metadata.IntentName != "app.stop" &&
	// 	// TODO: Figure out WTF and how to fix this mess
	// 	(hasAppToken ||
	// 		((input.Result.Metadata.IntentName == "app.initialize" ||
	// 			input.Result.Metadata.IntentName == "app.demo" ||
	// 			input.Result.Metadata.IntentName == "app.restart") &&
	// 			requestState.State.ProjectID != uuid.Nil)) {
	// 	contextOut = append(contextOut, *GenerateStateContext(GenerateStateTokenString(&requestState.State)))
	// }

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp":  time.Now().Add(time.Minute * 60 * 24 * 30).Unix(),
		"data": requestState.State,
	})
	// Sign the token and stringify
	tokenString, err := token.SignedString([]byte(os.Getenv("JWT_KEY")))
	if err != nil {
		log.Println("Error", err)
		return
	}

	response := aog.NewResponse(tokenString, requestState.OutputSSML.String(), requestState.OutputSSML.Raw(), true)
	response.ResponseMetadata["queryMatchInfo"] = struct {
		QueryMatched bool   `json:"queryMatched"`
		Intent       string `json:"intent"`
	}{
		true,
		parsedInput.Intent.Name,
	}

	// hasPreviousOutput := false
	// for _, ctx := range contextOut {
	// 	if ctx.Name != "previous_output" {
	// 		continue
	// 	}
	// 	hasPreviousOutput = true
	// }
	// if !hasPreviousOutput {
	// 	contextOut = append(contextOut,
	// 		actions.ApiAiContext{
	// 			Name: "previous_output",
	// 			Parameters: map[string]string{
	// 				// TODO: Make sure Shiva filters out special chars that would break this
	// 				// Namely '<'
	// 				"DisplayText": requestState.OutputSSML.Raw(),
	// 				"Speech":      requestState.OutputSSML.String(),
	// 			},
	// 			Lifespan: 1,
	// 		})
	// }

	// response.ContextOut = &contextOut

	json.NewEncoder(w).Encode(response)
}
