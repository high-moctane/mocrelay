package mocrelay

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestGetSessionID(t *testing.T) {
	ctx := context.Background()
	assert.Equal(t, "", GetSessionID(ctx))
	ctx = ctxWithSessionID(ctx)
	_, err := uuid.Parse(GetSessionID(ctx))
	assert.Nil(t, err)
}
