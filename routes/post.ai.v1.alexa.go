package routes

import (
	"net/http"

	"github.com/talkative-ai/go-alexa/skillserver"

	"github.com/talkative-ai/core/prehandle"
	"github.com/talkative-ai/core/router"
)

// GetProjects router.Route
// Path: "/user/register",
// Method: "GET",
// Accepts models.TokenValidate
// Responds with status of success or failure
var PostAlexa = &router.Route{
	Path:       "/ai/v1/alexa/{id:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}}",
	Method:     "POST",
	Prehandler: []prehandle.Prehandler{prehandle.SetJSON},
}

func PostAlexaHandler(w http.ResponseWriter, r *http.Request) {
	echoResp := skillserver.NewEchoResponse().OutputSpeech("Hello world").EndSession(true)
	json, _ := echoResp.String()
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	w.Write(json)
}
