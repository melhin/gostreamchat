package main

import (
	"context"
	"fmt"
	"gostreamchat/api"
	"log"
	"net/http"
	"os"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/mux"
)

var rdb *redis.Client
var ctx = context.Background()

func main() {

	redisURL, exists := os.LookupEnv("REDIS_URL")
	if !exists {
		panic("No Redis Url specified")
	}

	rdb := redis.NewClient(&redis.Options{Addr: redisURL})
	wd, err := os.Getwd()
	if err != nil {
		log.Fatalf("can not get os working directory: %v", err)
	}
	web := http.FileServer(http.Dir(wd + "/static"))

	r := mux.NewRouter()
	r.Path("/").Methods("GET").Handler(web)
	r.Path("/chat/{channel}").Methods("GET").HandlerFunc(api.H(rdb, api.ChatWebSocketHandler))
	r.Path("/user/{user}/channels").Methods("GET").HandlerFunc(api.H(rdb, api.UserChannelsHandler))
	r.Path("/users").Methods("GET").HandlerFunc(api.H(rdb, api.UsersHandler))

	port := ":" + os.Getenv("PORT")
	if port == ":" {
		port = ":8080"
	}
	fmt.Println("chat service started on port", port)
	log.Fatal(http.ListenAndServe(port, r))
}
