package routes

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	actions "github.com/talkative-ai/actions-on-google-golang/model"
	"github.com/talkative-ai/brahman/intent_handlers"
	"github.com/talkative-ai/core"
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

	input := &actions.ApiAiRequest{}
	err := json.NewDecoder(r.Body).Decode(input)
	if err != nil {
		log.Print("Error:", err)
		return
	}

	hasAppToken := false
	for _, ctx := range input.Result.Contexts {
		if !strings.HasPrefix(ctx.Name, "talkative_jwt_") {
			continue
		}
		claims, err := utilities.ParseJTWClaims(ctx.Parameters["token"])
		if err != nil {
			log.Print("Error:", err)
			return
		}
		stateMap := claims["state"].(map[string]interface{})

		requestState.State = models.MutableAIRequestState{
			Demo:      stateMap["Demo"].(bool),
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
		hasAppToken = true

		// TODO: Generalize this and create consistency between brahman/intent_handlers
		if stateMap["ZoneInitialized"] != nil {
			requestState.State.ZoneInitialized = map[uuid.UUID]bool{}
			for key, item := range stateMap["ZoneInitialized"].(map[string]interface{}) {
				requestState.State.ZoneInitialized[uuid.FromStringOrNil(key)] = item.(bool)
			}
		}
	}

	contextOut := []actions.ApiAiContext{}

	intentHandled := false
	if handler, ok := intentHandlers.List[input.Result.Metadata.IntentName]; ok {
		var ctx *[]actions.ApiAiContext
		ctx, err = handler(input, requestState)
		if err == nil {
			intentHandled = true
		}
		if err != nil && err != intentHandlers.ErrIntentNoMatch {
			fmt.Println("Error", err)
			return
		}
		if ctx != nil {
			contextOut = append(contextOut, *ctx...)
		}
	}

	if hasAppToken && !intentHandled {
		var ctx *[]actions.ApiAiContext
		ctx, err = intentHandlers.InappHandler(input, requestState)
		if err == nil {
			intentHandled = true
		}
		if err != nil && err != intentHandlers.ErrIntentNoMatch {
			fmt.Println("Error", err)
			return
		}
		if ctx != nil {
			contextOut = append(contextOut, *ctx...)
		}
	}

	if !intentHandled {
		_, err = intentHandlers.Unknown(input, requestState)
		if err != nil {
			fmt.Println("Error", err)
			return
		}
	}

	if input.Result.Metadata.IntentName != "app.stop" &&
		// TODO: Figure out WTF and how to fix this mess
		(hasAppToken ||
			((input.Result.Metadata.IntentName == "app.initialize" ||
				input.Result.Metadata.IntentName == "app.demo" ||
				input.Result.Metadata.IntentName == "app.restart") &&
				requestState.State.ProjectID != uuid.Nil)) {
		contextOut = append(contextOut, *GenerateStateContext(GenerateStateTokenString(&requestState.State)))
	}

	response := actions.ServiceResponse{
		DisplayText: requestState.OutputSSML.Raw(),
		Speech:      requestState.OutputSSML.String(),
	}

	hasPreviousOutput := false
	for _, ctx := range contextOut {
		if ctx.Name != "previous_output" {
			continue
		}
		hasPreviousOutput = true
	}
	if !hasPreviousOutput {
		contextOut = append(contextOut,
			actions.ApiAiContext{
				Name: "previous_output",
				Parameters: map[string]string{
					// TODO: Make sure Shiva filters out special chars that would break this
					// Namely '<'
					"DisplayText": requestState.OutputSSML.Raw(),
					"Speech":      requestState.OutputSSML.String(),
				},
				Lifespan: 1,
			})
	}

	response.ContextOut = &contextOut

	json.NewEncoder(w).Encode(response)
}

func GenerateStateContext(token string) *actions.ApiAiContext {
	tokenOut := actions.ApiAiContext{Name: fmt.Sprintf("talkative_jwt_%v", time.Now().UnixNano()), Parameters: map[string]string{"token": token}, Lifespan: 1}
	return &tokenOut
}

func GenerateStateTokenString(state *models.MutableAIRequestState) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"state": state,
	})

	tokenString, err := token.SignedString([]byte(os.Getenv("JWT_KEY")))
	if err != nil {
		log.Fatal("Error", err)
		return ""
	}
	return tokenString
}
