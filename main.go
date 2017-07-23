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

	"google.golang.org/appengine"

	apiai "github.com/artificial-universe-maker/apiai-go"
	"github.com/artificial-universe-maker/shiva/models"
)

type ActionHandler func(context.Context, *apiai.QueryResponse, *ResponseMessage)

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
		"input.welcome": welcome,
		"list.games":    listGames,
	}

	http.HandleFunc("/v1/", actionHandler)

	appengine.Main()
}

type ResponseMessage struct {
	Text string
}

func (r *ResponseMessage) Append(appended string) {
	if len(r.Text) == 0 {
		r.Text = appended
		return
	}
	r.Text = fmt.Sprintf("%s %s", r.Text, appended)
}

func (r *ResponseMessage) Prepare() (prepared map[string]string) {
	prepared = map[string]string{
		"speech":      r.Text,
		"displayText": r.Text,
	}
	return
}

func (r *ResponseMessage) IsEmpty() bool {
	return len(r.Text) == 0
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

	responseMessage := &ResponseMessage{}
	input := &apiai.QueryResponse{}

	w.Header().Add("content-type", "application/json")

	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(input)
	if err != nil {
		log.Println("Error", err)
		return
	}

	log.Println(input.Result.Action)

	if responseStrings, ok := RSIntro[input.Result.Action]; ok {
		responseMessage.Append(Choose(responseStrings))
	}

	if handler, ok := ActionHandlers[input.Result.Action]; ok {
		handler(r.Context(), input, responseMessage)
	}

	if responseMessage.IsEmpty() {
		unknown(r.Context(), input, responseMessage)
	}

	json.NewEncoder(w).Encode(responseMessage.Prepare())
}

func listGames(ctx context.Context, q *apiai.QueryResponse, message *ResponseMessage) {
	dsClient, _ := datastore.NewClient(ctx, "artificial-universe-maker")

	projects := make([]models.AumProject, 0)

	dsClient.GetAll(ctx, datastore.NewQuery("Project").Limit(1), &projects)

	message.Append(fmt.Sprintf(Choose(RSCustom["wrap new title"]), projects[0].Title))

	message.Append(Choose(RSCustom["hint actions after list.games"]))
}

func welcome(ctx context.Context, q *apiai.QueryResponse, message *ResponseMessage) {
	message.Append(Choose(RSCustom["introduce"]))
}

func unknown(ctx context.Context, q *apiai.QueryResponse, message *ResponseMessage) {
	message.Append(Choose(RSCustom["unknown"]))
}
