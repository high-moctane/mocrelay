package mocrelay

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestGetRequestID(t *testing.T) {
	ctx := context.Background()
	assert.Equal(t, "", GetRequestID(ctx))
	ctx = ctxWithRequestID(ctx)
	_, err := uuid.Parse(GetRequestID(ctx))
	assert.Nil(t, err)
}
