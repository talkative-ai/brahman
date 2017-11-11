package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/artificial-universe-maker/core/db"

	"github.com/artificial-universe-maker/core"

	actions "github.com/artificial-universe-maker/actions-on-google-golang/model"
	"github.com/artificial-universe-maker/brahman/intent_handlers"
	"github.com/artificial-universe-maker/go-ssml"
	"github.com/artificial-universe-maker/core/models"
	"github.com/artificial-universe-maker/core/providers"
	jwt "github.com/dgrijalva/jwt-go"
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
		log.Fatal("Error", err)
		return
	}

	hasGameToken := false
	for _, ctx := range input.Result.Contexts {
		if !strings.HasPrefix(ctx.Name, "aum_jwt_") {
			continue
		}
		claims, err := utilities.ParseJTWClaims(ctx.Parameters["token"])
		if err != nil {
			log.Fatal("Error", err)
			return
		}
		stateMap := claims["state"].(map[string]interface{})

		runtimeState.State = models.MutableRuntimeState{
			Zone:  stateMap["Zone"].(string),
			PubID: stateMap["PubID"].(string),
		}
		runtimeState.State.ZoneActors = map[string][]string{}
		for zone, actors := range stateMap["ZoneActors"].(map[string]interface{}) {
			runtimeState.State.ZoneActors[zone] = []string{}
			for _, actor := range actors.([]interface{}) {
				runtimeState.State.ZoneActors[zone] = append(runtimeState.State.ZoneActors[zone], actor.(string))
			}
		}
		if (stateMap["CurrentDialog"]) == nil {
			runtimeState.State.CurrentDialog = nil
		} else {
			s := stateMap["CurrentDialog"].(string)
			runtimeState.State.CurrentDialog = &s
		}
		hasGameToken = true
	}

	if hasGameToken {
		ingameHandler(input, runtimeState)
	} else if handler, ok := intentHandlers.List[input.Result.Metadata.IntentName]; ok {
		handler(input, runtimeState)
	} else {
		intentHandlers.Unknown(input, runtimeState)
	}

	response := actions.ServiceResponse{
		DisplayText: runtimeState.OutputSSML.String(),
		Speech:      runtimeState.OutputSSML.String(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"state": runtimeState.State,
	})

	tokenString, err := token.SignedString([]byte(os.Getenv("JWT_KEY")))
	if err != nil {
		log.Fatal("Error", err)
		return
	}

	tokenOut := actions.ApiAiContext{Name: fmt.Sprintf("aum_jwt_%v", time.Now().UnixNano()), Parameters: map[string]string{"token": tokenString}, Lifespan: 1}
	response.ContextOut = &[]actions.ApiAiContext{tokenOut}

	json.NewEncoder(w).Encode(response)
}

func ingameHandler(q *actions.ApiAiRequest, message *models.AumMutableRuntimeState) {
	redis, err := providers.ConnectRedis()
	if err != nil {
		log.Fatal("Error connecting to redis", err)
		return
	}
	projectID, err := strconv.ParseUint(message.State.PubID, 10, 64)
	if err != nil {
		log.Fatal("Error parsing projectID", err)
		return
	}
	err = db.InitializeDB()
	if err != nil {
		log.Fatal("Error parsing projectID", err)
		return
	}
	var dialogID string
	fmt.Printf("%+v", message.State)
	eventIDChan := make(chan uint64)
	go func() {
		var newID uint64
		err = db.Instance.QueryRow(`INSERT INTO event_user_action ("UserID", "PubID", "RawInput") VALUES ($1, $2, $3) RETURNING "ID"`, 1, projectID, q.Result.ResolvedQuery).Scan(&newID)
		if err != nil {
			// TODO: Log this error somewhere
			return
		}
		eventIDChan <- newID
	}()
	if message.State.CurrentDialog != nil {
		currentDialogKey := *message.State.CurrentDialog
		split := strings.Split(currentDialogKey, ":")
		currentDialogID, err := strconv.ParseUint(split[len(split)-1], 10, 64)
		if err != nil {
			log.Fatal("Error parsing current dialog ID", err)
			return
		}
		for _, actorIDString := range message.State.ZoneActors[message.State.Zone] {
			actorID, err := strconv.ParseUint(actorIDString, 10, 64)
			if err != nil {
				log.Fatal("Error parsing actorID", err)
				return
			}
			v := redis.HGet(models.KeynavCompiledDialogNodeWithinActor(projectID, actorID, currentDialogID), strings.ToUpper(q.Result.ResolvedQuery))
			if v.Err() == nil {
				dialogID = v.Val()
				break
			}
		}
	} else {
		for _, actorIDString := range message.State.ZoneActors[message.State.Zone] {
			actorID, err := strconv.ParseUint(actorIDString, 10, 64)
			if err != nil {
				log.Fatal("Error parsing actorID", err)
				return
			}
			v := redis.HGet(models.KeynavCompiledDialogRootWithinActor(projectID, actorID), strings.ToUpper(q.Result.ResolvedQuery))
			if v.Err() == nil {
				dialogID = v.Val()
				break
			}
		}
	}

	if dialogID == "" {
		intentHandlers.Unknown(q, message)
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
	}
	stateChange = true
	if stateChange {
		newID := <-eventIDChan
		stateObject, _ := message.State.Value()
		go db.Instance.QueryRow(`INSERT INTO event_state_change ("EventUserActionID", "StateObject") VALUES ($1, $2)`, newID, stateObject)
	}
}
