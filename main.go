package main

import (
	"log"
	"net/http"

	"github.com/artificial-universe-maker/brahman/routes"
	"github.com/artificial-universe-maker/core/router"

	"github.com/gorilla/mux"
)

func main() {

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})

	r := mux.NewRouter()
	router.ApplyRoute(r, routes.PostGoogle)
	router.ApplyRoute(r, routes.PostGoogleAuth)
	router.ApplyRoute(r, routes.PostGoogleAuthToken)

	http.Handle("/", r)

	log.Println("Brahman starting server on localhost:8080")

	log.Fatal(http.ListenAndServe(":8080", nil))
}
