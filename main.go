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
	db := NewDB(5)

	http.ListenAndServe("127.0.0.1:8234", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connID := uuid.NewString()

		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			log.Printf("[%v]: failed to upgrade http: %v", connID, err)
			return
		}
		defer conn.Close()

		if err := HandleWebsocket(r.Context(), r, connID, conn, router, db); err != nil {
			log.Printf("[%v]: websocket error: %v", connID, err)
		}
	}))
}
