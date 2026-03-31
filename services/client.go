package services

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/efficientip-labs/solidserver-go-client/sdsclient"
)

// APIClientWrapper wraps the sdsclient.APIClient to include credentials.
type APIClientWrapper struct {
	*sdsclient.APIClient
	tokenID     string
	tokenSecret string
}

// NewSolidServerClient initializes the SolidServer SDK client.
func NewSolidServerClient(host, tokenID, tokenSecret string, sslVerify bool) (*APIClientWrapper, error) {
	if host == "" || tokenID == "" || tokenSecret == "" {
		return nil, fmt.Errorf("missing SolidServer credentials: SOLIDSERVER_HOST, SOLIDSERVER_TOKEN_ID, and SOLIDSERVER_TOKEN_SECRET are required")
	}

	cfg := sdsclient.NewConfiguration()
	cfg.Servers = sdsclient.ServerConfigurations{
		{
			URL: fmt.Sprintf("https://%s/api/v2.0", host),
		},
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: !sslVerify,
			MinVersion:         tls.VersionTLS12,
		},
	}
	cfg.HTTPClient = &http.Client{Transport: tr}

	client := sdsclient.NewAPIClient(cfg)
	return &APIClientWrapper{
		APIClient:   client,
		tokenID:     tokenID,
		tokenSecret: tokenSecret,
	}, nil
}

// AuthContext returns a context with basic auth credentials (TokenID:TokenSecret).
func (c *APIClientWrapper) AuthContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, sdsclient.ContextBasicAuth, sdsclient.BasicAuth{
		UserName: c.tokenID,
		Password: c.tokenSecret,
	})
}
