package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/alextanhongpin/go-chat.v2/chat"
	"github.com/alextanhongpin/go-chat.v2/pkg/ticket"
	"github.com/julienschmidt/httprouter"
)

type contextKey string

const (
	port = 4000
)

func main() {
	issuer := ticket.New([]byte("secret :)"), 24*time.Hour)

	router := httprouter.New()
	router.NotFound = http.FileServer(http.Dir("public"))

	router.POST("/authenticate", newHandleAuthenticate(issuer))
	router.POST("/authorize", authorize(issuer, handleAuthorize))
	router.GET("/ws", func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		chat.ServeWs(w, r)
	})

	log.Printf("listening to port *:%d. press ctrl + c to cancel\n", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), router))
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

type Credential struct {
	AccessToken string `json:"accessToken"`
}

type User struct {
	Username string `json:"username"`
}

func handleAuthorize(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	user, ok := r.Context().Value(contextKey("user"))
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	u := User{user}
	if err := json.NewEncoder(w).Encode(u); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
}

func newHandleAuthenticate(t ticket.Issuer) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		var req User
		if err := json.NewDecoder(r).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		ticket, err := t.Issue(req.Username)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		res := Credential{
			AccessToken: ticket,
		}
		if err := json.NewEncoder(w).Encode(&res); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
}