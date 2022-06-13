package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/alextanhongpin/go-chat.v2/domain"
	"github.com/alextanhongpin/go-chat.v2/infra"
	"github.com/alextanhongpin/go-chat.v2/pkg/chat"
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

	c, close := chat.New("chat", redis, issuer)
	defer close()

	handleWs := func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		c.ServeWS(w, r)
	}

	router.GET("/ws", handleWs)

	log.Printf("listening to port *:%d. press ctrl + c to cancel\n", port)
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: router,
	}
	Server(srv)
}

type authorizer interface {
	Issue(subject string) (string, error)
	Verify(token string) (string, error)
}

func authorize(t authorizer, next httprouter.Handle) httprouter.Handle {
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

func newHandleAuthenticate(t authorizer) httprouter.Handle {
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

func Server(srv *http.Server) {
	// Create context that listens for the interrupt signal from the OS.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	// Initializing the server in a goroutine so that
	// it won't block the graceful shutdown handling below
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	// Listen for the interrupt signal.
	<-ctx.Done()

	// Restore default behavior on the interrupt signal and notify user of shutdown.
	stop()
	log.Println("shutting down gracefully, press Ctrl+C again to force")

	// The context is used to inform the server it has 5 seconds to finish
	// the request it is currently handling
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown: ", err)
	}

	log.Println("Server exiting")
}
