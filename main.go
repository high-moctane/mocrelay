package main

import (
	"log"
	"net/http"
	"os"

	"github.com/gobwas/ws"
	"github.com/google/uuid"
)

var logStdout = log.New(os.Stdout, "I: ", log.Default().Flags())
var logStderr = log.New(os.Stderr, "E: ", log.Default().Flags())

func main() {
	log.Printf("server start")

	router := new(Router)

	http.ListenAndServe(":80", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connID := uuid.NewString()

		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			log.Printf("[%v]: failed to upgrade http: %v", connID, err)
			return
		}

		HandleWebsocket(r.Context(), r, connID, conn, router)
	}))
}
