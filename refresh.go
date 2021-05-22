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

type AsyncRefreshingTokenSource struct {
	genFunc func(ctx context.Context) (oauth2.TokenSource, error)
	token   *oauth2.Token
	conf    AsyncRefreshingConfig
	mu      sync.Mutex
}

func (ts *AsyncRefreshingTokenSource) Token() (*oauth2.Token, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.token, nil
}

// AsyncRefreshingConfig is the refresh configuration of NewAsyncRefreshingTokenSource.
type AsyncRefreshingConfig struct {
	// BeforeExpiryMargin A is the margin for refreshing the token before Expiry.
	// If it is zero value, TokenSource don't care about Expiry.
	BeforeExpiryMargin time.Duration
	// RefreshInterval is interval for refreshing token if Expiry based refreshing is not applied.
	// If not set, 10 minutes is default interval.
	RefreshInterval time.Duration
	// JitterFunc is jitter function for RefreshInterval and BeforeExpiryMargin.
	// It is compatible with https://pkg.go.dev/github.com/lthibault/jitterbug#Jitter.
	JitterFunc interface {
		Jitter(time.Duration) time.Duration
	}
}

// NewAsyncRefreshingTokenSource create TokenSource with the refresh config conf and the TokenSource generator function genFunc.
// genFunc will be called to generate the one-time TokenSource instance every time to refresh.
func NewAsyncRefreshingTokenSource(ctx context.Context, conf AsyncRefreshingConfig, genFunc func(ctx context.Context) (oauth2.TokenSource, error)) (oauth2.TokenSource, error) {
	if conf.RefreshInterval == 0 {
		conf.RefreshInterval = defaultInterval
	}
	if conf.JitterFunc == nil {
		conf.JitterFunc = jitterbug.Norm{}
	}
	b := &AsyncRefreshingTokenSource{genFunc: genFunc, conf: conf}
	expiry, err := b.flip(ctx)
	if err != nil {
		return nil, err
	}
	go b.run(ctx, expiry)
	return b, nil
}

func (ts *AsyncRefreshingTokenSource) flip(ctx context.Context) (time.Time, error) {
	tokenSource, err := ts.genFunc(ctx)
	if err != nil {
		return time.Time{}, err
	}
	t, err := tokenSource.Token()
	if err != nil {
		return time.Time{}, err
	}

	ts.mu.Lock()
	ts.token = t
	ts.mu.Unlock()
	return t.Expiry, nil
}

func (ts *AsyncRefreshingTokenSource) run(ctx context.Context, initialExpiry time.Time) {
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

	waitUntilExpiryC := handleExpiry(initialExpiry)

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
