package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/acme/autocert"

	"github.com/artificial-universe-maker/go-utilities/db"

	"github.com/artificial-universe-maker/go-utilities"

	actions "github.com/artificial-universe-maker/actions-on-google-golang/model"
	"github.com/artificial-universe-maker/brahman/helpers"
	"github.com/artificial-universe-maker/go-ssml"
	"github.com/artificial-universe-maker/go-utilities/keynav"
	"github.com/artificial-universe-maker/go-utilities/models"
	"github.com/artificial-universe-maker/go-utilities/providers"
	jwt "github.com/dgrijalva/jwt-go"
)

type ActionHandler func(*actions.ApiAiRequest, *models.AumMutableRuntimeState)

var RSIntro map[string][]string
var RSCustom map[string][]string
var ActionHandlers map[string]ActionHandler

func main() {

	RSIntro = map[string][]string{
		"input.welcome": []string{
			"Hi there!",
			"Hey!",
		},
		"list.games": []string{
			"There's a ton of games.",
		},
	}

	RSCustom = map[string][]string{
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

	ActionHandlers = map[string]ActionHandler{
		"input.welcome":   welcome,
		"list.games":      listGames,
		"game.initialize": initializeGame,
	}

	http.HandleFunc("/v1/google", actionHandler)
	http.HandleFunc("/v1/google/auth", googleAuthHandler)
	http.HandleFunc("/v1/google/auth.token", googleAuthTokenHandler)

	log.Println("Brahman starting server on localhost:8080")
	m := autocert.Manager{
		Cache:      autocert.DirCache("secret-dir"),
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist("api.aum.ai"),
		Email:      "info@aum.ai",
	}
	s := &http.Server{
		Addr:      ":8080",
		TLSConfig: &tls.Config{GetCertificate: m.GetCertificate},
	}
	log.Fatal(s.ListenAndServeTLS("", ""))
}

const (
	AuthGoogleClientID       = "558300683184-vqt364nq9hko57c81gia7fkiclkt1ste.apps.googleusercontent.com"
	AuthGoogleRedirectURI    = "https://oauth-redirect.googleusercontent.com/r/artificial-universe-make-7ef2b"
	AuthGoogleDevRedirectURI = "https://developers.google.com/oauthplayground"
)

func googleAuthTokenHandler(w http.ResponseWriter, r *http.Request) {

}

func googleAuthHandler(w http.ResponseWriter, r *http.Request) {

	clientID := r.FormValue("client_id")
	if clientID != AuthGoogleClientID {
		fmt.Println("Invalid client id", clientID)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	redirectURI := r.FormValue("redirect_uri")
	if redirectURI != AuthGoogleRedirectURI && redirectURI != AuthGoogleDevRedirectURI {
		fmt.Println("Invalid redirect URI", redirectURI)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	stateString := r.FormValue("state")

	pwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	if data, err := ioutil.ReadFile(pwd + "/auth.html"); err == nil {
		template := strings.Replace(string(data), "${state}", fmt.Sprintf(`"%v"`, stateString), 1)
		template = strings.Replace(template, "${redirectURI}", fmt.Sprintf(`"%v"`, redirectURI), 1)
		w.Write([]byte(template))
	} else {
		panic(err)
	}
}

func Prepare(msg ssml.Builder) (prepared map[string]string) {
	prepared = map[string]string{
		"speech":      msg.String(),
		"displayText": msg.String(),
	}
	return
}

func pseudoRand(max int) int {
	rand.Seed(time.Now().UTC().UnixNano())
	return rand.Intn(max)
}

func Choose(list []string) string {
	if len(list) == 1 {
		return list[0]
	}
	return list[pseudoRand(len(list))]
}

func actionHandler(w http.ResponseWriter, r *http.Request) {

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
		log.Println("Error", err)
		return
	}

	hasGameToken := false
	for _, ctx := range input.Result.Contexts {
		if !strings.HasPrefix(ctx.Name, "aum_jwt_") {
			continue
		}
		claims, err := utilities.ParseJTWClaims(ctx.Parameters["token"])
		if err != nil {
			log.Println("Error", err)
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
		fmt.Println("%+v", runtimeState)
		hasGameToken = true
	}

	if hasGameToken {
		ingameHandler(input, runtimeState)
	} else {
		if responseStrings, ok := RSIntro[input.Result.Metadata.IntentName]; ok {
			runtimeState.OutputSSML = runtimeState.OutputSSML.Text(Choose(responseStrings))
		} else if handler, ok := ActionHandlers[input.Result.Metadata.IntentName]; ok {
			handler(input, runtimeState)
		} else {
			unknown(input, runtimeState)
		}
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
		log.Println("Error", err)
		return
	}

	tokenOut := actions.ApiAiContext{Name: fmt.Sprintf("aum_jwt_%v", time.Now().UnixNano()), Parameters: map[string]string{"token": tokenString}, Lifespan: 1}
	response.ContextOut = &[]actions.ApiAiContext{tokenOut}

	json.NewEncoder(w).Encode(response)
}

func listGames(q *actions.ApiAiRequest, message *models.AumMutableRuntimeState) {
	message.OutputSSML = message.OutputSSML.Text(Choose(RSCustom["hint actions after list.games"]))
}

func welcome(q *actions.ApiAiRequest, message *models.AumMutableRuntimeState) {
	message.OutputSSML = message.OutputSSML.Text(Choose(RSCustom["introduce"]))
}

func unknown(q *actions.ApiAiRequest, message *models.AumMutableRuntimeState) {
	message.OutputSSML = message.OutputSSML.Text(Choose(RSCustom["unknown"]))
}

func initializeGame(q *actions.ApiAiRequest, message *models.AumMutableRuntimeState) {
	redis, err := providers.ConnectRedis()
	if err != nil {
		log.Println("Error", err)
		return
	}
	projectIDString := redis.HGet(keynav.GlobalMetaProjects(), strings.ToUpper(q.Result.Parameters["game"])).Val()
	projectID, err := strconv.ParseUint(projectIDString, 10, 64)
	if err != nil {
		log.Println("Error", err)
		return
	}
	zoneID := redis.HGet(keynav.ProjectMetadataStatic(uint64(projectID)), "start_zone_id").Val()
	message.State.Zone = zoneID
	message.State.PubID = projectIDString
	message.State.ZoneActors = map[string][]string{}
	for _, zidString := range redis.SMembers(
		fmt.Sprintf("%v:%v", keynav.ProjectMetadataStatic(uint64(projectID)), "all_zones")).Val() {
		zid, err := strconv.ParseUint(zidString, 10, 64)
		if err != nil {
			log.Println("Error", err)
			return
		}
		message.State.ZoneActors[zidString] =
			redis.SMembers(keynav.CompiledActorsWithinZone(projectID, zid)).Val()
	}
	message.OutputSSML = message.OutputSSML.Text(fmt.Sprintf("Okay, starting %v. Have fun!", q.Result.Parameters["game"]))
}

func ingameHandler(q *actions.ApiAiRequest, message *models.AumMutableRuntimeState) {
	redis, err := providers.ConnectRedis()
	if err != nil {
		log.Println("Error connecting to redis", err)
		return
	}
	projectID, err := strconv.ParseUint(message.State.PubID, 10, 64)
	if err != nil {
		log.Println("Error parsing projectID", err)
		return
	}
	db.InitializeDB()
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
			log.Println("Error parsing current dialog ID", err)
			return
		}
		for _, actorIDString := range message.State.ZoneActors[message.State.Zone] {
			actorID, err := strconv.ParseUint(actorIDString, 10, 64)
			if err != nil {
				log.Println("Error parsing actorID", err)
				return
			}
			v := redis.HGet(keynav.CompiledDialogNodeWithinActor(projectID, actorID, currentDialogID), strings.ToUpper(q.Result.ResolvedQuery))
			if v.Err() == nil {
				dialogID = v.Val()
				break
			}
		}
	} else {
		for _, actorIDString := range message.State.ZoneActors[message.State.Zone] {
			actorID, err := strconv.ParseUint(actorIDString, 10, 64)
			if err != nil {
				log.Println("Error parsing actorID", err)
				return
			}
			v := redis.HGet(keynav.CompiledDialogRootWithinActor(projectID, actorID), strings.ToUpper(q.Result.ResolvedQuery))
			if v.Err() == nil {
				dialogID = v.Val()
				break
			}
		}
	}

	if dialogID == "" {
		unknown(q, message)
		return
	}

	dialogBinary, err := redis.Get(dialogID).Bytes()
	if err != nil {
		log.Println("Error fetching logic binary", dialogID, err)
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
	result := helpers.LogicLazyEval(stateComms, dialogBinary)
	for res := range result {
		if res.Error != nil {
			log.Println("Error with logic evaluation", res.Error)
			return
		}
		bundleBinary, err := redis.Get(res.Value).Bytes()
		if err != nil {
			log.Println("Error fetching action bundle binary", err)
			return
		}
		err = helpers.ActionBundleEval(message, bundleBinary)
		if err != nil {
			log.Println("Error processing action bundle binary", err)
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
