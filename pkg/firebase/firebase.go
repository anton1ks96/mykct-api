// Package firebase поднимает клиент FCM из Firebase Admin SDK.
package firebase

import (
	"context"
	"fmt"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"
)

// NewMessagingClient создаёт клиент FCM по JSON-ключу сервисного аккаунта.
func NewMessagingClient(ctx context.Context, credentialsPath string) (*messaging.Client, error) {
	app, err := firebase.NewApp(ctx, nil, option.WithCredentialsFile(credentialsPath))
	if err != nil {
		return nil, fmt.Errorf("failed to init firebase app: %w", err)
	}

	client, err := app.Messaging(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to init firebase messaging: %w", err)
	}

	return client, nil
}
