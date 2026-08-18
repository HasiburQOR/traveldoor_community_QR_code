package handlers

import (
	"context"

	"traveldoor/qrprofile/internal/models"
)

type ctxKey string

const userKey ctxKey = "user"

func withUser(ctx context.Context, u *models.User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

func userFrom(ctx context.Context) *models.User {
	u, _ := ctx.Value(userKey).(*models.User)
	return u
}
