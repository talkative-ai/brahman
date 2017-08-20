package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"cloud.google.com/go/datastore"

	actions "github.com/artificial-universe-maker/actions-on-google-golang/model"
	"github.com/artificial-universe-maker/go-ssml"
	"github.com/artificial-universe-maker/go-utilities/models"
)

type ActionHandler func(context.Context, *actions.ApiAiRequest, *models.AumMutableRuntimeState)

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

	http.ListenAndServe(":8085", nil)
}

const (
	AuthGoogleClientID       = "558300683184-vqt364nq9hko57c81gia7fkiclkt1ste.apps.googleusercontent.com"
	AuthGoogleRedirectURI    = "https://oauth-redirect.googleusercontent.com/r/artificial-universe-make-7ef2b"
	AuthGoogleDevRedirectURI = "https://developers.google.com/oauthplayground"
)

func googleAuthTokenHandler(w http.ResponseWriter, r *http.Request) {

}

func googleAuthHandler(w http.ResponseWriter, r *http.Request) {
	//https://brahman.ngrok.io/v1/google/auth?response_type=token&client_id=558300683184-vqt364nq9hko57c81gia7fkiclkt1ste.apps.googleusercontent.com&redirect_uri=https://oauth-redirect.googleusercontent.com/r/artificial-universe-make-7ef2b&scope=email+name&state=CoIDQUZEXzV0a2FxNDhCZkpIeUxZaGVTc2otQkFfVlRib2VpNnhCeGdsOF91UmROWkF2LXNoV1JwQkNhN1F5alRIc2pYWkN2bURneUxGNVluOFVhNlRabkUtSkRWaUpFOXdGS1ZBemhpQlQ2R2ZycEhkMDRPVnFTbzIxbVRaNVQ2U2M1eUpFLS0xNHpyVXRaS055eVk5UW9WQ3BJeVRYOFhjMGxsbFltY1VPLVRaN0NsQnk2b0FONVdONmlYVW1Mdko2bEppQkhGYVlYdTViUml4ZEt5VV9EajhvUHlwN185aWx2czRHdnNuTVhUMWx3dDQ2akhMVWpyeldjcUtKTU1tUmotTFNVa2tfeXpUVVo2aEpJZ0t3elNpSTZrWUpzekhZSkstVVBQX0M2eE5kNmpXV0Z5TklLNWxWV1VVWjNYTG1qRmVPVk9nejRkVzBya2E5MVQzc1VWeE1QZXZUektyZGRTM0Q5Y2hwaEtrQ0ZKeVFGbk16R2d6aFI1QmZvUElmTUESF2h0dHBzOi8vd3d3Lmdvb2dsZS5jb20vIk1odHRwczovL29hdXRoLXJlZGlyZWN0Lmdvb2dsZXVzZXJjb250ZW50LmNvbS9yL2FydGlmaWNpYWwtdW5pdmVyc2UtbWFrZS03ZWYyYjIiYXJ0aWZpY2lhbC11bml2ZXJzZS1tYWtlLTdlZjJiX2Rldg
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

	for k, v := range r.Form {
		fmt.Println("Key:", k, "Val:", v)
	}

	http.Redirect(w, r, fmt.Sprintf("%v#access_token=123&token_type=bearer&state=%v", redirectURI, stateString), 302)
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

	w.Header().Add("content-type", "application/json")

	runtimeState := &models.AumMutableRuntimeState{
		State:      map[string]string{},
		OutputSSML: ssml.NewBuilder(),
	}

	// // Save a copy of this request for debugging.
	// requestDump, err := httputil.DumpRequest(r, true)
	// if err != nil {
	// 	fmt.Println(err)
	// }
	// fmt.Println(string(requestDump))

	input := &actions.ApiAiRequest{}
	err := json.NewDecoder(r.Body).Decode(input)
	if err != nil {
		log.Println("Error", err)
		return
	}

	if responseStrings, ok := RSIntro[input.Result.Metadata.IntentName]; ok {
		runtimeState.OutputSSML = runtimeState.OutputSSML.Text(Choose(responseStrings))
	} else if handler, ok := ActionHandlers[input.Result.Metadata.IntentName]; ok {
		handler(r.Context(), input, runtimeState)
	} else {
		unknown(r.Context(), input, runtimeState)
	}

	fmt.Printf("%+v", input.Result.Contexts)

	response := actions.ServiceResponse{
		DisplayText: runtimeState.OutputSSML.String(),
		Speech:      runtimeState.OutputSSML.String(),
	}
	out := actions.ApiAiContext{Name: fmt.Sprintf("aum_jwt_%v", time.Now().UnixNano()), Parameters: map[string]string{"token": fmt.Sprintf("%v", time.Now().UnixNano())}, Lifespan: 1}
	response.ContextOut = &[]actions.ApiAiContext{out}

	json.NewEncoder(w).Encode(response)
}

func listGames(ctx context.Context, q *actions.ApiAiRequest, message *models.AumMutableRuntimeState) {
	dsClient, _ := datastore.NewClient(ctx, "artificial-universe-maker")

	projects := make([]models.AumProject, 0)

	dsClient.GetAll(ctx, datastore.NewQuery("Project").Limit(1), &projects)

	message.OutputSSML = message.OutputSSML.Text(fmt.Sprintf(Choose(RSCustom["wrap new title"]), projects[0].Title))

	message.OutputSSML = message.OutputSSML.Text(Choose(RSCustom["hint actions after list.games"]))

}

func welcome(ctx context.Context, q *actions.ApiAiRequest, message *models.AumMutableRuntimeState) {
	message.OutputSSML = message.OutputSSML.Text(Choose(RSCustom["introduce"]))
}

func unknown(ctx context.Context, q *actions.ApiAiRequest, message *models.AumMutableRuntimeState) {
	message.OutputSSML = message.OutputSSML.Text(Choose(RSCustom["unknown"]))
}

func initializeGame(ctx context.Context, q *actions.ApiAiRequest, message *models.AumMutableRuntimeState) {
	message.OutputSSML = message.OutputSSML.Text(Choose(RSCustom["introduce"]))
}
