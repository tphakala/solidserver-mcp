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
	username string
	password string
}

// NewSolidServerClient initializes the SolidServer SDK client.
func NewSolidServerClient(host, username, password string, sslVerify bool) (*APIClientWrapper, error) {
	if host == "" || username == "" || password == "" {
		return nil, fmt.Errorf("missing SolidServer credentials: SOLIDSERVER_HOST, SOLIDSERVER_USERNAME, and SOLIDSERVER_PASSWORD are required")
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
		APIClient: client,
		username:  username,
		password:  password,
	}, nil
}

// AuthContext returns a context with basic auth credentials.
func (c *APIClientWrapper) AuthContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, sdsclient.ContextBasicAuth, sdsclient.BasicAuth{
		UserName: c.username,
		Password: c.password,
	})
}
