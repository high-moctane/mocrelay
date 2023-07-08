package main

import (
	"log"
	"time"

	jsoniter "github.com/json-iterator/go"
)

const (
	AccessLogConnect    = "CONNECT"
	AccessLogDisconnect = "DISCONNECT"
	AccessLogSend       = "Send"
	AccessLogRecv       = "Recv"
)

var accessLog *log.Logger

func DoAccessLog(addr, connID, what, content string) {
	ji := jsoniter.ConfigCompatibleWithStandardLibrary

	v := AccessLog{
		At:           time.Now(),
		RemoteAddr:   addr,
		ConnectionID: connID,
		What:         what,
		Content:      content,
	}
	s, _ := ji.MarshalToString(&v)

	logStdout.Println(s)
}

type AccessLog struct {
	At           time.Time `json:"at"`
	RemoteAddr   string    `json:"remote_addr"`
	ConnectionID string    `json:"conn_id"`
	What         string    `json:"what"`
	Content      string    `json:"content,omitempty"`
}
