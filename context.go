package mocrelay

import (
	"context"
	"net/http"
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
