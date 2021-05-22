package tokensource

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/lthibault/jitterbug/v2"
	"golang.org/x/oauth2"
)

const defaultInterval = 10 * time.Minute

type forceRefreshTokenSource struct {
	genFunc     func(ctx context.Context) (oauth2.TokenSource, error)
	tokenSource oauth2.TokenSource
	conf        ForceRefreshConfig
	mu          sync.RWMutex
}

func (ts *forceRefreshTokenSource) Token() (*oauth2.Token, error) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.tokenSource.Token()
}

type ForceRefreshConfig struct {
	// BeforeExpiryMargin A is the margin for refreshing the token before Expiry.
	// If it is zero value, TokenSource don't care about Expiry.
	BeforeExpiryMargin time.Duration
	// RefreshInterval is interval for refreshing token if Expiry based refreshing is not applied.
	RefreshInterval time.Duration
	// JitterFunc is jitter function for RefreshInterval and BeforeExpiryMargin.
	// It is compatible with https://pkg.go.dev/github.com/lthibault/jitterbug#Jitter.
	JitterFunc interface {
		Jitter(time.Duration) time.Duration
	}
}

func NewForceRefreshTokenSource(ctx context.Context, conf ForceRefreshConfig, genFunc func(ctx context.Context) (oauth2.TokenSource, error)) (oauth2.TokenSource, error) {
	if conf.RefreshInterval == 0 {
		conf.RefreshInterval = defaultInterval
	}
	if conf.JitterFunc == nil {
		conf.JitterFunc = jitterbug.Norm{}
	}
	b := &forceRefreshTokenSource{genFunc: genFunc, conf: conf}
	if _, err := b.flip(ctx); err != nil {
		return nil, err
	}
	go b.run(ctx)
	return b, nil
}

func (ts *forceRefreshTokenSource) flip(ctx context.Context) (time.Time, error) {
	tokenSource, err := ts.genFunc(ctx)
	if err != nil {
		return time.Time{}, err
	}
	t, err := tokenSource.Token()
	if err != nil {
		return time.Time{}, err
	}

	ts.mu.Lock()
	ts.tokenSource = tokenSource
	ts.mu.Unlock()
	return t.Expiry, nil
}

func (ts *forceRefreshTokenSource) run(ctx context.Context) {
	ticker := jitterbug.New(ts.conf.RefreshInterval, ts.conf.JitterFunc)
	defer ticker.Stop()

	handleExpiry := func(expiry time.Time) <-chan time.Time {
		if ts.conf.BeforeExpiryMargin != 0 && !expiry.IsZero() {
			targetTime := expiry.Add(-ts.conf.JitterFunc.Jitter(ts.conf.BeforeExpiryMargin))
			return time.After(time.Until(targetTime))
		} else {
			return nil
		}
	}

	token, _ := ts.Token()
	waitUntilExpiryC := handleExpiry(token.Expiry)

loop:
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// ignore ticker during waiting until expiry
			if waitUntilExpiryC != nil {
				continue loop
			}
		case <-waitUntilExpiryC:
		}

		expiry, err := ts.flip(ctx)
		if err != nil {
			// TODO: Implement exponential backoff and don't panic
			log.Fatalln(err)
		}
		waitUntilExpiryC = handleExpiry(expiry)
	}
}
