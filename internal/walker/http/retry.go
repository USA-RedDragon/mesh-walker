package http

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"syscall"
	"time"

	"github.com/USA-RedDragon/mesh-walker/internal/apimodels"
)

// StatusError is a response with a status other than 200 OK.
type StatusError struct {
	StatusCode int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("received status code %d", e.StatusCode)
}

// permanent reports whether a retry would get the same answer: the host
// refused the connection, has no route, has no DNS name, or said 404.
func permanent(err error) bool {
	var dnsErr *net.DNSError
	return IsNotFound(err) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.EHOSTUNREACH) ||
		errors.Is(err, syscall.ENETUNREACH) ||
		(errors.As(err, &dnsErr) && dnsErr.IsNotFound)
}

// IsNotFound reports whether err is a 404 response.
func IsNotFound(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && se.StatusCode == http.StatusNotFound
}

type Client struct {
	client  http.Client
	retries int
	jitter  time.Duration
}

func NewClient(timeout, dialTimeout time.Duration, retries int, jitter time.Duration) *Client {
	dialer := &net.Dialer{
		Timeout:   dialTimeout,
		KeepAlive: -1,
	}
	return &Client{
		client: http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				DisableKeepAlives: true,
				DialContext:       dialer.DialContext,
			},
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if req.URL.Path == "/a/sysinfo" {
					return nil
				}
				return http.ErrUseLastResponse
			},
		},
		retries: retries,
		jitter:  jitter,
	}
}

func (c *Client) jitterSleep() {
	//nolint:gosec
	time.Sleep(time.Duration(rand.Int63n(int64(c.jitter))))
}

func (c *Client) Get(ctx context.Context, url string) (*apimodels.SysinfoResponse, error) {
	var resp *http.Response
	c.jitterSleep()

	for n := range c.retries {
		var err error
		resp, err = c.get(ctx, url)
		if err != nil {
			if permanent(err) {
				return nil, err
			}
			if n == c.retries-1 {
				return nil, fmt.Errorf("failed to get url after %d retries: %w", c.retries, err)
			}
			c.jitterSleep()
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			return nil, &StatusError{StatusCode: resp.StatusCode}
		}
		if resp.StatusCode != http.StatusOK {
			if n == c.retries-1 {
				return nil, fmt.Errorf("after %d retries: %w", c.retries, &StatusError{StatusCode: resp.StatusCode})
			}
			c.jitterSleep()
			continue
		}
		break
	}

	var response apimodels.SysinfoResponse
	if err := response.Decode(resp.Body); err != nil {
		return nil, fmt.Errorf("failed to decode response body: %w", err)
	}

	return &response, nil
}

func (c *Client) get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get error: %w", err)
	}

	return resp, nil
}
