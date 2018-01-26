package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/artificial-universe-maker/brahman/routes"
	"github.com/artificial-universe-maker/core/db"
	"github.com/artificial-universe-maker/core/redis"
	"github.com/artificial-universe-maker/core/router"

	"github.com/gorilla/mux"
)

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
	router.ApplyRoute(r, routes.PostGoogleAuth)
	router.ApplyRoute(r, routes.PostGoogleAuthToken)

	http.Handle("/", r)

	log.Println("Brahman starting server on localhost:8080")

	log.Fatal(http.ListenAndServe(":8080", nil))
}
