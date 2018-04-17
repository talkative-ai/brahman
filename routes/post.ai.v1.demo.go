package routes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/gorilla/mux"
	"github.com/talkative-ai/aog"
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

		// TODO: This should not duplicate the existing intent handler code when starting an app

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

		response := aog.NewResponse("", message.OutputSSML.String(), message.OutputSSML.Raw(), false)
		responseBytes, err := json.Marshal(response.ExpectedInputs)
		if err != nil {
			myerrors.Respond(w, &myerrors.MySimpleError{
				Code:    http.StatusBadRequest,
				Message: "marshal error",
				Req:     r,
				Log:     err.Error(),
			})
			return
		}
		message.State.PreviousResponse = string(responseBytes)

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"exp":  time.Now().Add(time.Minute * 3).Unix(),
			"data": message.State,
		})
		// Sign the token and stringify
		tokenString, err := token.SignedString([]byte(os.Getenv("JWT_KEY")))
		if err != nil {
			myerrors.Respond(w, &myerrors.MySimpleError{
				Code:    http.StatusBadRequest,
				Message: "tokenString signing problem",
				Req:     r,
				Log:     err.Error(),
			})
			return
		}
		output.SSML = message.OutputSSML.String()
		output.Text = message.OutputSSML.Raw()
		output.State = &tokenString

	} else {

		req := aog.Request{
			Conversation: aog.Conversation{
				ConversationID:    "Demo",
				Type:              "ACTIVE",
				ConversationToken: *input.State,
			},
			Inputs: []aog.Input{
				aog.Input{
					Intent: aog.ConstIntentText,
					RawInputs: []aog.RawInput{
						aog.RawInput{
							InputType: aog.ConstInputTypeKeyboard,
							Query:     input.Message,
						},
					},
					Arguments: []aog.InputArgument{
						aog.InputArgument{
							Name:      aog.ConstInputArgumentText,
							RawText:   input.Message,
							TextValue: input.Message,
						},
					},
				},
			},
		}

		payload, err := json.Marshal(req)
		if err != nil {
			myerrors.Respond(w, &myerrors.MySimpleError{
				Code:    http.StatusBadRequest,
				Message: "marshal error",
				Req:     r,
				Log:     err.Error(),
			})
			return
		}
		rq, err := http.NewRequest("POST", fmt.Sprintf("http://127.0.0.1:8080/ai/v1/google"), bytes.NewReader(payload))

		client := http.Client{}
		resp, err := client.Do(rq)

		rawResponse, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			myerrors.Respond(w, &myerrors.MySimpleError{
				Code:    http.StatusBadRequest,
				Message: "request error",
				Req:     r,
				Log:     err.Error(),
			})
			return
		}

		response := aog.Response{}

		json.Unmarshal(rawResponse, &response)

		output.State = &response.ConversationToken
		if len(response.ExpectedInputs) > 0 &&
			len(response.ExpectedInputs[0].InputPrompt.RichInitialPrompt.Items) > 0 &&
			response.ExpectedInputs[0].InputPrompt.RichInitialPrompt.Items[0].(map[string]interface{}) != nil {
			output.SSML = response.ExpectedInputs[0].InputPrompt.RichInitialPrompt.Items[0].(map[string]interface{})["simpleResponse"].(map[string]interface{})["ssml"].(string)
			output.Text = response.ExpectedInputs[0].InputPrompt.RichInitialPrompt.Items[0].(map[string]interface{})["simpleResponse"].(map[string]interface{})["displayText"].(string)
		} else {
			myerrors.Respond(w, &myerrors.MySimpleError{
				Code:    http.StatusBadRequest,
				Message: "insufficient brahman response",
				Req:     r,
				Log:     fmt.Sprintf("%+v", response),
			})
			return
		}

	}

	json.NewEncoder(w).Encode(output)

}
