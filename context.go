package main

import (
	"context"
	"net/http"

	"github.com/tomasen/realip"
)

type CtxKeyConnID struct{}

func GetCtxConnID(ctx context.Context) string {
	return ctx.Value(CtxKeyConnID{}).(string)
}

func SetCtxConnID(ctx context.Context, connID string) context.Context {
	return context.WithValue(ctx, CtxKeyConnID{}, connID)
}

type CtxKeyRealIP struct{}

func GetCtxRealIP(ctx context.Context) string {
	return ctx.Value(CtxKeyRealIP{}).(string)
}

func SetCtxRealIPFronRequest(ctx context.Context, r *http.Request) context.Context {
	return context.WithValue(ctx, CtxKeyRealIP{}, realip.FromRequest(r))
}
