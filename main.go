package main

import (
	"log"
	"net/http"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

func main() {
	log.Printf("start")
	http.ListenAndServe("127.0.0.1:8234", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			log.Printf("failed to upgrade http: %v", err)
			return
		}
		go func() {
			defer conn.Close()

			for {
				msg, op, err := wsutil.ReadClientData(conn)
				if err != nil {
					log.Printf("failed to read client data: %v", err)
					return
				}
				err = wsutil.WriteServerMessage(conn, op, msg)
				if err != nil {
					log.Printf("failed to write server message: %v", err)
					return
				}
			}
		}()
	}))
}
