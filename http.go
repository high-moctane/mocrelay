package main

import "net/http"

func RealIP(r *http.Request) string {
	if addr := r.Header.Get("X-Real-IP"); addr != "" {
		return addr
	} else if addr := r.Header.Get("X-Forwarder-For"); addr != "" {
		return addr
	}
	return r.RemoteAddr
}
