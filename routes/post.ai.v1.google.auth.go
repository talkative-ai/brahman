package routes

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/artificial-universe-maker/core/prehandle"
	"github.com/artificial-universe-maker/core/router"
)

// GetProjects router.Route
// Path: "/user/register",
// Method: "GET",
// Accepts models.TokenValidate
// Responds with status of success or failure
var PostGoogleAuth = &router.Route{
	Path:       "/ai/v1/google/auth/token",
	Method:     "POST",
	Handler:    http.HandlerFunc(postGoogleAuthHandler),
	Prehandler: []prehandle.Prehandler{prehandle.SetJSON},
}

func postGoogleAuthHandler(w http.ResponseWriter, r *http.Request) {

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
