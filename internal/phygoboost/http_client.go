package phygoboost

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	boostnetwork "github.com/KiriKirby/phytozome-go/internal/phygoboost/network"
)

func newSharedHTTPClient() *http.Client {
	profile := Current()
	base := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          maxInt(16, profile.MaxIdleConns),
		MaxIdleConnsPerHost:   maxInt(4, profile.MaxIdlePerHost),
		IdleConnTimeout:       profile.IdleConnTimeout,
		TLSHandshakeTimeout:   profile.TLSHandshake,
		ExpectContinueTimeout: profile.ExpectContinue,
		DialContext: (&net.Dialer{
			Timeout:   minDurationPositive(profile.TLSHandshake, 10*time.Second),
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}
	transport := &http.Transport{
		Proxy:                 base.Proxy,
		ForceAttemptHTTP2:     base.ForceAttemptHTTP2,
		MaxIdleConns:          base.MaxIdleConns,
		MaxIdleConnsPerHost:   base.MaxIdleConnsPerHost,
		IdleConnTimeout:       base.IdleConnTimeout,
		TLSHandshakeTimeout:   base.TLSHandshakeTimeout,
		ExpectContinueTimeout: base.ExpectContinueTimeout,
		DialContext:           base.DialContext,
	}
	return &http.Client{
		Timeout:   minDurationPositive(profile.HTTPTimeout, 60*time.Second),
		Transport: &domainAwareTransport{base: transport},
	}
}

func minDurationPositive(value time.Duration, fallback time.Duration) time.Duration {
	if value <= 0 {
		return fallback
	}
	return value
}

type domainAwareTransport struct {
	base http.RoundTripper
}

func (t *domainAwareTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, errors.New("nil http request")
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	domain := ""
	if req.URL != nil {
		domain = strings.TrimSpace(req.URL.Hostname())
		if domain == "" {
			domain = strings.TrimSpace(req.URL.Host)
		}
	}
	if contextHasNetworkGrant(contextForRequest(req), domain) {
		started := time.Now()
		resp, err := base.RoundTrip(req)
		return observeExistingGrantResponse(domain, started, resp, err)
	}
	request := ResourceRequest{
		ManagedLevel: ExecManaged,
		ManagedSlots: 1,
		Network:      map[string]int{domain: 1},
		Description:  "implicit shared http request",
	}
	handle, err := DeclareResources(contextForRequest(req), request)
	if err != nil {
		return nil, err
	}
	runCtx := BindDeclaredResources(contextForRequest(req), handle)
	req = req.Clone(runCtx)
	started := time.Now()
	resp, err := base.RoundTrip(req)
	if err != nil {
		handle.releasers[0](time.Since(started), err, false)
		handle.releasers = nil
		handle.Release()
		return nil, err
	}
	rateLimited := resp.StatusCode == http.StatusTooManyRequests
	serverErr := resp.StatusCode >= 500
	if resp.Body == nil {
		if len(handle.releasers) > 0 {
			handle.releasers[0](time.Since(started), statusError(resp, err), rateLimited || serverErr)
			handle.releasers = nil
		}
		handle.Release()
		return resp, nil
	}
	resp.Body = &networkReleaseBody{
		ReadCloser: resp.Body,
		release: func(latency time.Duration, err error, rateLimited bool) {
			if len(handle.releasers) > 0 {
				handle.releasers[0](latency, err, rateLimited)
				handle.releasers = nil
			}
			handle.Release()
		},
		started:     started,
		rateLimited: rateLimited || serverErr,
		statusCode:  resp.StatusCode,
	}
	return resp, nil
}

func observeExistingGrantResponse(domain string, started time.Time, resp *http.Response, err error) (*http.Response, error) {
	latency := time.Since(started)
	if err != nil {
		ObserveDomainResult(domain, latency, err, false, 0)
		return nil, err
	}
	rateLimited := resp != nil && boostnetwork.IsRetryableStatus(resp)
	cooldown := boostnetwork.RetryAfterDelay(resp)
	if resp == nil || resp.Body == nil {
		ObserveDomainResult(domain, latency, statusError(resp, err), rateLimited, cooldown)
		return resp, nil
	}
	resp.Body = &networkObserveBody{
		ReadCloser:  resp.Body,
		domain:      domain,
		started:     started,
		rateLimited: rateLimited,
		statusCode:  resp.StatusCode,
		cooldown:    cooldown,
	}
	return resp, nil
}

type networkReleaseBody struct {
	io.ReadCloser
	once        sync.Once
	release     func(time.Duration, error, bool)
	started     time.Time
	rateLimited bool
	statusCode  int
}

func (b *networkReleaseBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(func() {
		if b.release != nil {
			b.release(time.Since(b.started), statusErrorCode(b.statusCode, err), b.rateLimited)
		}
	})
	return err
}

type networkObserveBody struct {
	io.ReadCloser
	once        sync.Once
	domain      string
	started     time.Time
	rateLimited bool
	statusCode  int
	cooldown    time.Duration
}

func (b *networkObserveBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(func() {
		ObserveDomainResult(b.domain, time.Since(b.started), statusErrorCode(b.statusCode, err), b.rateLimited, b.cooldown)
	})
	return err
}

func contextForRequest(req *http.Request) context.Context {
	if req == nil {
		return context.Background()
	}
	if ctx := req.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

func statusError(resp *http.Response, err error) error {
	if err != nil {
		return err
	}
	if resp == nil {
		return nil
	}
	return statusErrorCode(resp.StatusCode, nil)
}

func statusErrorCode(statusCode int, err error) error {
	if err != nil {
		return err
	}
	if statusCode == http.StatusTooManyRequests || statusCode >= 500 {
		return errors.New(http.StatusText(statusCode))
	}
	return nil
}
