package firebase

import (
	"context"
	"encoding/base64"
	"fmt"

	fb "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"google.golang.org/api/option"
	"cloud.google.com/go/firestore"

	"drexa/internal/config"
)

type Client struct {
	App       *fb.App
	Auth      *auth.Client
	Firestore *firestore.Client
}

func New(ctx context.Context, cfg config.FirebaseConfig) (*Client, error) {
	appCfg := &fb.Config{ProjectID: cfg.ProjectID}

	var opts []option.ClientOption
	if cfg.CredentialsJSON != "" {
		creds, err := base64.StdEncoding.DecodeString(cfg.CredentialsJSON)
		if err != nil {
			return nil, fmt.Errorf("failed to decode firebase credentials: %w", err)
		}
		opts = append(opts, option.WithCredentialsJSON(creds))
	}

	app, err := fb.NewApp(ctx, appCfg, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize firebase app: %w", err)
	}

	authClient, err := app.Auth(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize firebase auth: %w", err)
	}

	firestoreClient, err := app.Firestore(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize firestore: %w", err)
	}

	return &Client{App: app, Auth: authClient, Firestore: firestoreClient}, nil
}
