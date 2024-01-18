package mocrelay

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/tomasen/realip"
)

type requestKeyType struct{}

var requestKey = requestKeyType{}

func ctxWithRequest(ctx context.Context, r *http.Request) context.Context {
	return context.WithValue(ctx, requestKey, r)
}

func GetRequest(ctx context.Context) *http.Request {
	r, ok := ctx.Value(requestKey).(*http.Request)
	if !ok {
		return nil
	}
	return r
}

type requestIDKeyType struct{}

var requestIDKey = requestIDKeyType{}

func ctxWithRequestID(ctx context.Context) context.Context {
	return context.WithValue(ctx, requestIDKey, uuid.NewString())
}

func GetRequestID(ctx context.Context) string {
	id, ok := ctx.Value(requestIDKey).(string)
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

type httpHeaderKeyType struct{}

var httpHeaderKey = httpHeaderKeyType{}

func ctxWithHTTPHeader(ctx context.Context, r *http.Request) context.Context {
	return context.WithValue(ctx, httpHeaderKey, r.Header)
}

func GetHTTPHeader(ctx context.Context) http.Header {
	header, ok := ctx.Value(httpHeaderKey).(http.Header)
	if !ok {
		return nil
	}
	return header
}
