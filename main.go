package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/artificial-universe-maker/go.uuid"
	jwt "github.com/dgrijalva/jwt-go"

	"github.com/artificial-universe-maker/core"

	actions "github.com/artificial-universe-maker/actions-on-google-golang/model"
	"github.com/artificial-universe-maker/brahman/intent_handlers"
	"github.com/artificial-universe-maker/core/models"
	"github.com/artificial-universe-maker/go-ssml"
)

func main() {

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})

	http.HandleFunc("/ai/v1/google", AIRequestHandler)
	http.HandleFunc("/ai/v1/google/auth", googleAuthHandler)
	http.HandleFunc("/ai/v1/google/auth.token", googleAuthTokenHandler)

	log.Println("Brahman starting server on localhost:8080")

	log.Fatal(http.ListenAndServe(":8080", nil))
}

func googleAuthTokenHandler(w http.ResponseWriter, r *http.Request) {
	// TODO: Verify if the user exists. If not, create account and sign in, else sign in
}

func googleAuthHandler(w http.ResponseWriter, r *http.Request) {

	stateString := r.FormValue("state")

	pwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	if data, err := ioutil.ReadFile(pwd + "/auth.html"); err == nil {
		template := strings.Replace(string(data), "${state}", fmt.Sprintf(`"%v"`, stateString), 1)
		template = strings.Replace(template, "${redirectURI}", fmt.Sprintf(`"%v"`, os.Getenv("AuthGoogleRedirectURI")), 1)
		w.Write([]byte(template))
	} else {
		panic(err)
	}
}

// AIRequestHandler handles requests that expect language parsing and an AI response
// Currently expects ApiAi requests
// This is the core functionality of Brahman, which routes to appropriate IntentHandlers
func AIRequestHandler(w http.ResponseWriter, r *http.Request) {

	if os.Getenv("REDIS_ADDR") == "" {
		os.Setenv("REDIS_ADDR", "127.0.0.1:6379")
		os.Setenv("REDIS_PASSWORD", "")
	}

	w.Header().Add("content-type", "application/json")

	runtimeState := &models.AumMutableRuntimeState{
		State:      models.MutableRuntimeState{},
		OutputSSML: ssml.NewBuilder(),
	}

	input := &actions.ApiAiRequest{}
	err := json.NewDecoder(r.Body).Decode(input)
	if err != nil {
		log.Print("Error:", err)
		return
	}

	hasGameToken := false
	for _, ctx := range input.Result.Contexts {
		if !strings.HasPrefix(ctx.Name, "aum_jwt_") {
			continue
		}
		claims, err := utilities.ParseJTWClaims(ctx.Parameters["token"])
		if err != nil {
			log.Print("Error:", err)
			return
		}
		stateMap := claims["state"].(map[string]interface{})

		runtimeState.State = models.MutableRuntimeState{
			Zone:  uuid.FromStringOrNil(stateMap["Zone"].(string)),
			PubID: uuid.FromStringOrNil(stateMap["PubID"].(string)),
		}
		runtimeState.State.ZoneActors = map[uuid.UUID][]string{}
		if stateMap["ZoneActors"] != nil {
			for zone, actors := range stateMap["ZoneActors"].(map[string]interface{}) {
				runtimeState.State.ZoneActors[uuid.FromStringOrNil(zone)] = []string{}
				for _, actor := range actors.([]interface{}) {
					runtimeState.State.ZoneActors[uuid.FromStringOrNil(zone)] = append(runtimeState.State.ZoneActors[uuid.FromStringOrNil(zone)], actor.(string))
				}
			}
		}
		if (stateMap["CurrentDialog"]) == nil {
			runtimeState.State.CurrentDialog = nil
		} else {
			s := stateMap["CurrentDialog"].(string)
			runtimeState.State.CurrentDialog = &s
		}
		hasGameToken = true

		// TODO: Generalize this and create consistency between brahman/intent_handlers
		if stateMap["ZoneInitialized"] != nil {
			runtimeState.State.ZoneInitialized = map[uuid.UUID]bool{}
			for key, item := range stateMap["ZoneInitialized"].(map[string]interface{}) {
				runtimeState.State.ZoneInitialized[uuid.FromStringOrNil(key)] = item.(bool)
			}
		}
	}

	contextOut := []actions.ApiAiContext{}

	intentHandled := false
	if handler, ok := intentHandlers.List[input.Result.Metadata.IntentName]; ok {
		var ctx *[]actions.ApiAiContext
		ctx, err = handler(input, runtimeState)
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

	if hasGameToken && !intentHandled {
		var ctx *[]actions.ApiAiContext
		ctx, err = intentHandlers.IngameHandler(input, runtimeState)
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
		_, err = intentHandlers.Unknown(input, runtimeState)
		if err != nil {
			fmt.Println("Error", err)
			return
		}
	}

	if input.Result.Metadata.IntentName != "app.stop" &&
		(hasGameToken ||
			((input.Result.Metadata.IntentName == "app.initialize" || input.Result.Metadata.IntentName == "app.restart") &&
				runtimeState.State.PubID != uuid.Nil)) {
		contextOut = append(contextOut, *generateStateToken(runtimeState))
	}

	response := actions.ServiceResponse{
		DisplayText: runtimeState.OutputSSML.Raw(),
		Speech:      runtimeState.OutputSSML.String(),
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
					"DisplayText": runtimeState.OutputSSML.Raw(),
					"Speech":      runtimeState.OutputSSML.String(),
				},
				Lifespan: 1,
			})
	}

	response.ContextOut = &contextOut

	json.NewEncoder(w).Encode(response)
}

func generateStateToken(runtimeState *models.AumMutableRuntimeState) *actions.ApiAiContext {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"state": runtimeState.State,
	})

	tokenString, err := token.SignedString([]byte(os.Getenv("JWT_KEY")))
	if err != nil {
		log.Fatal("Error", err)
		return nil
	}

	tokenOut := actions.ApiAiContext{Name: fmt.Sprintf("aum_jwt_%v", time.Now().UnixNano()), Parameters: map[string]string{"token": tokenString}, Lifespan: 1}
	return &tokenOut
}
