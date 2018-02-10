package routes

import (
	"net/http"

	"github.com/talkative-ai/core/prehandle"
	"github.com/talkative-ai/core/router"
)

// GetProjects router.Route
// Path: "/user/register",
// Method: "GET",
// Accepts models.TokenValidate
// Responds with status of success or failure
var PostGoogleAuthToken = &router.Route{
	Path:       "/ai/v1/google/auth/token",
	Method:     "POST",
	Handler:    http.HandlerFunc(postGoogleAuthTokenHandler),
	Prehandler: []prehandle.Prehandler{prehandle.SetJSON},
}

func postGoogleAuthTokenHandler(w http.ResponseWriter, r *http.Request) {
	// TODO: Verify if the user exists. If not, create account and sign in, else sign in
}
