package routes

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/talkative-ai/core/models"
	ssml "github.com/talkative-ai/go-ssml"

	"github.com/talkative-ai/go.uuid"

	"github.com/talkative-ai/core/myerrors"
	"github.com/talkative-ai/core/prehandle"
	"github.com/talkative-ai/core/router"
)

// GetProjects router.Route
// Path: "/interface",
// Method: "GET",
// Accepts models.TokenValidate
// Responds with status of success or failure
var PostDemo = &router.Route{
	Path:       "/ai/v1/demo/{id:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}}",
	Method:     "POST",
	Handler:    http.HandlerFunc(postDemoHander),
	Prehandler: []prehandle.Prehandler{prehandle.SetJSON, prehandle.JWT, prehandle.RequireBody(65535)},
}

type postDemoInput struct {
	Message string
	State   *string
}
type postDemoOutput struct {
	SSML  string
	Text  string
	State *string
}

// AIRequestHandler handles requests that expect language parsing and an AI response
// Currently expects ApiAi requests
// This is the core functionality of Brahman, which routes to appropriate IntentHandlers
func postDemoHander(w http.ResponseWriter, r *http.Request) {

	var input postDemoInput
	output := postDemoOutput{}

	err := json.Unmarshal([]byte(r.Header.Get("X-Body")), &input)
	if err != nil {
		myerrors.ServerError(w, r, err)
		return
	}

	if input.State == nil {
		// Parse the project ID from the URL
		urlparams := mux.Vars(r)
		projectID, err := uuid.FromString(urlparams["id"])
		if err != nil {
			myerrors.Respond(w, &myerrors.MySimpleError{
				Code:    http.StatusBadRequest,
				Message: "bad_id",
				Req:     r,
			})
			return
		}
		var setup models.RAResetApp
		message := models.AIRequest{
			State:      models.MutableAIRequestState{},
			OutputSSML: ssml.NewBuilder(),
		}
		message.State = models.MutableAIRequestState{}
		message.State.Demo = true
		message.State.SessionID = uuid.NewV4()
		message.State.ProjectID = projectID
		message.State.PubID = fmt.Sprintf("demo:%v", projectID.String())
		setup.Execute(&message)

		output.SSML = message.OutputSSML.String()
		output.Text = message.OutputSSML.Raw()
	} else {

		// claims, err := utilities.ParseJTWClaims(*input.State)
		// if err != nil {
		// 	log.Print("Error:", err)
		// 	return
		// }
		// stateMap := claims["state"].(map[string]interface{})

		// client, err := apiai.NewClient(
		// 	&apiai.ClientConfig{
		// 		Token:      "83e827c573d54af89f994e18ebc1f279",
		// 		QueryLang:  "en",
		// 		SpeechLang: "en-US",
		// 	},
		// )
		// if err != nil {
		// 	myerrors.ServerError(w, r, err)
		// 	return
		// }
		// context := GenerateStateContext(*input.State)
		// //Set the query string and your current user identifier.
		// // TODO: Stop using two different libraries here
		// ctx := apiai.Context{}
		// ctx.Name = context.Name
		// ctx.Lifespan = context.Lifespan
		// ctx.Params = map[string]interface{}{}
		// for key, val := range context.Parameters {
		// 	ctx.Params[key] = val
		// }
		// qr, err := client.Query(apiai.Query{Query: []string{input.Message}, Contexts: []apiai.Context{ctx}, SessionId: stateMap["SessionID"].(string)})
		// if err != nil {
		// 	myerrors.ServerError(w, r, err)
		// 	return
		// }
		// output.SSML = qr.Result.Fulfillment.Speech
		// output.Text = qr.Result.Fulfillment.DisplayText
		// for _, ctx := range qr.Result.Contexts {
		// 	if !strings.HasPrefix(ctx.Name, "talkative_jwt_") {
		// 		continue
		// 	}
		// 	tkn := ctx.Params["token"].(string)
		// 	output.State = &tkn
		// }
	}

	json.NewEncoder(w).Encode(output)

}
