package mocrelay

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/tomasen/realip"
)

type sessionIDKeyType struct{}

var sessionIDKey = sessionIDKeyType{}

func ctxWithSessionID(ctx context.Context) context.Context {
	return context.WithValue(ctx, sessionIDKey, uuid.NewString())
}

func GetSessionID(ctx context.Context) string {
	id, ok := ctx.Value(sessionIDKey).(string)
	if !ok {
		return ""
	}
	return id
}

type realIPKeyType struct{}

var realIPKey = realIPKeyType{}

func ctxWithRealIP(ctx context.Context, r *http.Request) context.Context {
	return context.WithValue(ctx, realIPKey, realip.FromRequest(r))
}

func GetRealIP(ctx context.Context) string {
	ip, ok := ctx.Value(realIPKey).(string)
	if !ok {
		return ""
	}
	return ip
}
