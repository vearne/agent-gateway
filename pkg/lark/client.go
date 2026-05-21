package lark

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
)

const larkHTTPTimeout = 120 * time.Second

func newAPIClient(appID, appSecret string) *lark.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSHandshakeTimeout = 30 * time.Second
	transport.ResponseHeaderTimeout = 60 * time.Second
	transport.ExpectContinueTimeout = 1 * time.Second
	transport.IdleConnTimeout = 90 * time.Second
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 10

	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	transport.DialContext = dialer.DialContext

	httpClient := &http.Client{
		Timeout:   larkHTTPTimeout,
		Transport: transport,
	}

	return lark.NewClient(appID, appSecret, lark.WithHttpClient(httpClient))
}

func isRetryableNetErr(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "TLS handshake timeout") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "EOF") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "i/o timeout")
}

func retryOnNetErr(ctx context.Context, fn func() error) error {
	const attempts = 3
	var lastErr error
	for i := 0; i < attempts; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		lastErr = fn()
		if lastErr == nil || !isRetryableNetErr(lastErr) {
			return lastErr
		}
		if i < attempts-1 {
			time.Sleep(time.Duration(i+1) * time.Second)
		}
	}
	return lastErr
}
