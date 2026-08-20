package services

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/efficientip-labs/solidserver-go-client/sdsclient"
	"golang.org/x/time/rate"
)

const (
	defaultHTTPClientTimeout = 30 * time.Second
	defaultMaxRetriesCount   = 3
	defaultInitBackoff       = 200 * time.Millisecond
	defaultMaxBackoffLimit   = 10 * time.Second
)

// ClientOptions holds configuration options for the SolidServer client.
type ClientOptions struct {
	Host        string
	TokenID     string
	TokenSecret string
	SSLVerify   bool
	HTTPTimeout time.Duration
	MaxRetries  int
	RateLimit   float64 // Requests per second (0 = unlimited)
}

// APIClientWrapper wraps the sdsclient.APIClient to include credentials and metadata.
type APIClientWrapper struct {
	*sdsclient.APIClient
	tokenID     string
	tokenSecret string
	host        string
	sslVerify   bool
}

// Host returns the configured SOLIDserver hostname.
func (c *APIClientWrapper) Host() string {
	return c.host
}

// SSLVerify returns whether TLS verification is enabled.
func (c *APIClientWrapper) SSLVerify() bool {
	return c.sslVerify
}

// NewSolidServerClientWithOptions initializes the SolidServer SDK client with full options.
//
//nolint:gocritic // opts by value is idiomatic for small client configuration structs
func NewSolidServerClientWithOptions(opts ClientOptions) (*APIClientWrapper, error) {
	if opts.Host == "" || opts.TokenID == "" || opts.TokenSecret == "" {
		return nil, fmt.Errorf("missing SolidServer credentials: SOLIDSERVER_HOST, SOLIDSERVER_TOKEN_ID, and SOLIDSERVER_TOKEN_SECRET are required")
	}

	cfg := sdsclient.NewConfiguration()
	cfg.Servers = sdsclient.ServerConfigurations{
		{
			URL: fmt.Sprintf("https://%s/api/v2.0", opts.Host),
		},
	}

	var baseTr *http.Transport
	if defTr, ok := http.DefaultTransport.(*http.Transport); ok {
		baseTr = defTr.Clone()
	} else {
		baseTr = &http.Transport{}
	}
	baseTr.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: !opts.SSLVerify,
		MinVersion:         tls.VersionTLS12,
	}

	var limiter *rate.Limiter
	if opts.RateLimit > 0 {
		limiter = rate.NewLimiter(rate.Limit(opts.RateLimit), int(opts.RateLimit)+1)
	}

	retryTr := &RetryTransport{
		Base:           baseTr,
		MaxRetries:     opts.MaxRetries,
		InitialBackoff: defaultInitBackoff,
		MaxBackoff:     defaultMaxBackoffLimit,
		Limiter:        limiter,
	}

	timeout := opts.HTTPTimeout
	if timeout <= 0 {
		timeout = defaultHTTPClientTimeout
	}

	cfg.HTTPClient = &http.Client{
		Transport: retryTr,
		Timeout:   timeout,
	}

	return &APIClientWrapper{
		APIClient:   sdsclient.NewAPIClient(cfg),
		tokenID:     opts.TokenID,
		tokenSecret: opts.TokenSecret,
		host:        opts.Host,
		sslVerify:   opts.SSLVerify,
	}, nil
}

// NewSolidServerClient initializes the SolidServer SDK client with default options.
func NewSolidServerClient(host, tokenID, tokenSecret string, sslVerify bool) (*APIClientWrapper, error) {
	return NewSolidServerClientWithOptions(ClientOptions{
		Host:        host,
		TokenID:     tokenID,
		TokenSecret: tokenSecret,
		SSLVerify:   sslVerify,
		HTTPTimeout: defaultHTTPClientTimeout,
		MaxRetries:  defaultMaxRetriesCount,
		RateLimit:   0,
	})
}

// AuthContext returns a context with EIP API Token credentials.
func (c *APIClientWrapper) AuthContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, sdsclient.ContextEipApiTokenAuth, sdsclient.EipApiTokenAuth{
		Token:  c.tokenID,
		Secret: c.tokenSecret,
	})
}
