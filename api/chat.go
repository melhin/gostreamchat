package api

import (
	"encoding/json"
	"fmt"
	"gostreamchat/user"
	"net/http"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

var upgrader websocket.Upgrader

var connectedUsers = make(map[string]*user.User)

func H(rdb *redis.Client, fn func(http.ResponseWriter, *http.Request, *redis.Client)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		fn(w, r, rdb)
	}
}

type msg struct {
	Message string `json:"message,omitempty"`
}

const (
	commandSubscribe = iota
	commandUnsubscribe
	commandChat
)

func ChatWebSocketHandler(w http.ResponseWriter, r *http.Request, rdb *redis.Client) {

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		handleWSError(err, conn)
		return
	}

	err = onConnect(r, conn, rdb)
	if err != nil {
		handleWSError(err, conn)
		return
	}

	closeCh := onDisconnect(r, conn, rdb)

	onChannelMessage(conn, r)

loop:
	for {
		select {
		case <-closeCh:
			break loop
		default:
			onUserMessage(conn, r, rdb)
		}
	}
}

func onConnect(r *http.Request, conn *websocket.Conn, rdb *redis.Client) error {
	username := r.URL.Query()["username"][0]
	channel := mux.Vars(r)["channel"]
	fmt.Println("connected from:", conn.RemoteAddr(), "user:", username)

	u, err := user.Connect(rdb, username, channel)
	if err != nil {
		return err
	}
	connectedUsers[username] = u
	return nil
}

func onDisconnect(r *http.Request, conn *websocket.Conn, rdb *redis.Client) chan struct{} {

	closeCh := make(chan struct{})

	username := r.URL.Query()["username"][0]

	conn.SetCloseHandler(func(code int, text string) error {
		fmt.Println("connection closed for user", username)

		u := connectedUsers[username]
		if err := u.Disconnect(); err != nil {
			return err
		}
		delete(connectedUsers, username)
		close(closeCh)
		return nil
	})

	return closeCh
}

func onUserMessage(conn *websocket.Conn, r *http.Request, rdb *redis.Client) {

	var msg msg

	username := r.URL.Query()["username"][0]
	if err := conn.ReadJSON(&msg); err != nil {
		handleWSError(err, conn)
		return
	}

	u := connectedUsers[username]

	detail := user.DetailMsg{
		Sender:      username,
		Message:     msg.Message,
		MessageType: user.Message,
	}
	if err := user.Chat(rdb, u.Channel, detail); err != nil {
		handleWSError(err, conn)
	}
}

func onChannelMessage(conn *websocket.Conn, r *http.Request) {

	username := r.URL.Query()["username"][0]
	u := connectedUsers[username]

	go func() {
		for m := range u.MessageChan {
			msg := &user.DetailMsg{}
			if err := json.Unmarshal([]byte(m.Payload), msg); err != nil {
				fmt.Println(err)
			}

			if err := conn.WriteJSON(msg); err != nil {
				fmt.Println(err)
			}
		}

	}()
}

func handleWSError(err error, conn *websocket.Conn) {
	_ = conn.WriteJSON(msg{Message: err.Error()})
}
