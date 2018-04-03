package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/rs/cors"
	"github.com/talkative-ai/brahman/routes"
	"github.com/talkative-ai/core/db"
	"github.com/talkative-ai/core/redis"
	"github.com/talkative-ai/core/router"
	"github.com/talkative-ai/go-alexa/skillserver"

	"github.com/gorilla/mux"
)

func wrapRequest(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL)
		handler.ServeHTTP(w, r)
	})
}
func main() {

	// Initialize database and redis connections
	// TODO: Make it a bit clearer that this is happening, and more maintainable
	err := db.InitializeDB()
	if err != nil {
		fmt.Println(err)
		return
	}
	defer db.Instance.Close()

	_, err = redis.ConnectRedis()
	if err != nil {
		fmt.Println(err)
		return
	}
	defer redis.Instance.Close()

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})

	r := mux.NewRouter()
	router.ApplyRoute(r, routes.PostGoogle)
	router.ApplyRoute(r, routes.PostDemo)
	router.ApplyRoute(r, routes.PostGoogleAuth)
	router.ApplyRoute(r, routes.PostGoogleAuthToken)

	skillserver.SetEchoPrefix("/ai/v1/alexa/")
	skillserver.Init(map[string]interface{}{
		routes.PostAlexa.Path: skillserver.EchoApplication{
			Handler: routes.PostAlexaHandler,
		},
	}, r)

	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"https://talkative.ai", "https://harihara.ngrok.io", "http://brahman.ngrok.io", "https://brahman.ngrok.io", "https://workbench.talkative.ai", "http://localhost:3000", "http://localhost:8080", "http://localhost:3001"},
		AllowCredentials: true,
		AllowedHeaders:   []string{"x-token", "accept", "content-type"},
		ExposedHeaders:   []string{"etag", "x-token"},
		AllowedMethods:   []string{"GET", "PATCH", "POST", "PUT"},
	})

	http.Handle("/", c.Handler(r))

	log.Println("Brahman starting server on localhost:8080")

	log.Fatal(http.ListenAndServe(":8080", wrapRequest(http.DefaultServeMux)))
}
