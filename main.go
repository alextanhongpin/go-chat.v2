package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"strings"
	"sync"
	"time"

	"github.com/alextanhongpin/go-chat.v2/domain"
	"github.com/alextanhongpin/go-chat.v2/infra"
	"github.com/alextanhongpin/go-chat.v2/pkg/socketio"
	"github.com/alextanhongpin/go-chat.v2/pkg/ticket"
	"github.com/julienschmidt/httprouter"
)

type contextKey string

const (
	port = 3000
)

type Message struct {
	// From: user id, To: group id
	From, To string
	Type     string
	Text     string
}

func main() {
	redis := infra.NewRedis()
	// NOTE: Separate the token for authorizing user, vs token for authorizing
	// websocket connection.
	// For simplicity, we use the same for both.
	issuer := ticket.New([]byte("secret :)"), 24*time.Hour)

	router := httprouter.New()
	router.POST("/authenticate", newHandleAuthenticate(issuer))
	router.POST("/authorize", authorize(issuer, handleAuthorize))
	router.NotFound = http.FileServer(http.Dir("public"))
	router.Handler(http.MethodGet, "/debug/pprof/*item", http.DefaultServeMux)

	io, close := socketio.NewIORedis[Message]("chat", redis)
	defer close()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		// Listen to the redis pubsub server for new messages.
		for msg := range io.Subscribe() {
			// Send to the local server if exists.
			io.Emit(msg.To, msg)
			fmt.Println("subscribe:", msg)
		}
	}()

	handleWs := func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		token := r.URL.Query().Get("token")
		username, err := issuer.Verify(token)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		log.Printf("ws: logged in as %s\n", username)

		socket, err, close := io.Connect(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer close()

		ctx := r.Context()

		// Find the list of friends, and notify them on the online status.
		//io.Publish(ctx, Message{
		//Type: "presence",
		//From: socket.ID,
		//To: ,
		//})

		// Listen to the local client for messages.
		for msg := range socket.Listen() {
			switch msg.Type {
			case "text":
				// PUblish to all redis server.
				io.Publish(ctx, msg)
			default:
				fmt.Println("not handled", msg.Type)
				io.Publish(ctx, msg)
			}
		}
	}

	router.GET("/ws", handleWs)

	log.Printf("listening to port *:%d. press ctrl + c to cancel\n", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), router))
	wg.Wait()
}

func authorize(t ticket.Issuer, next httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		auth := r.Header.Get("Authorization")
		token := strings.ReplaceAll(auth, "Bearer ", "")
		user, err := t.Verify(token)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), contextKey("user"), user)
		r = r.WithContext(ctx)
		next(w, r, ps)
	}
}

func handleAuthorize(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	user, ok := r.Context().Value(contextKey("user")).(string)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	u := domain.User{Username: user}
	if err := json.NewEncoder(w).Encode(u); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
}

func newHandleAuthenticate(t ticket.Issuer) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		var req domain.User
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		ticket, err := t.Issue(req.Username)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		res := domain.Credential{
			AccessToken: ticket,
		}
		if err := json.NewEncoder(w).Encode(&res); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
}
