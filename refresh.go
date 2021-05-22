package tokensource

import (
	"context"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
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
	// MarginBeforeExpiry is the margin for refreshing the token before Expiry.
	// If it is zero value, TokenSource don't care about Expiry.
	MarginBeforeExpiry time.Duration
	// RandomizationFactorForRefreshInterval is randomization factor for MarginBeforeExpiry.
	RandomizationFactorForMarginBeforeExpiry float64

	// RefreshInterval is interval for refreshing token if Expiry based refreshing is not applied.
	// If not set, 10 minutes is default interval.
	RefreshInterval time.Duration
	// RandomizationFactorForRefreshInterval is randomization factor for RefreshInterval.
	RandomizationFactorForRefreshInterval float64

	// Backoff is backoff configuration for TokenSource.Token().
	// If not set, backoff.NewExponentialBackOff is used as the default value.
	// See also https://pkg.go.dev/github.com/cenkalti/backoff/v4#NewExponentialBackOff.
	Backoff backoff.BackOff
}

// NewAsyncRefreshingTokenSource create TokenSource with the refresh config conf and the TokenSource generator function genFunc.
// genFunc will be called to generate the one-time TokenSource instance every time to refresh.
// Note: NewAsyncRefreshingTokenSource fetches the first token synchronously.
func NewAsyncRefreshingTokenSource(ctx context.Context, conf AsyncRefreshingConfig, genFunc func(ctx context.Context) (oauth2.TokenSource, error)) (oauth2.TokenSource, error) {
	if conf.RefreshInterval == 0 {
		conf.RefreshInterval = defaultInterval
	}
	if conf.Backoff == nil {
		conf.Backoff = backoff.NewExponentialBackOff()
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
	var token *oauth2.Token
	err := backoff.Retry(func() error {
		tokenSource, err := ts.genFunc(ctx)
		if err != nil {
			return err
		}

		t, err := tokenSource.Token()
		if err != nil {
			return err
		}
		token = t
		return nil
	}, ts.conf.Backoff)
	if err != nil {
		return time.Time{}, err
	}

	ts.mu.Lock()
	ts.token = token
	ts.mu.Unlock()
	return token.Expiry, nil
}

func (ts *AsyncRefreshingTokenSource) run(ctx context.Context, initialExpiry time.Time) {
	ticker := tickerWithJitter(ts.conf.RefreshInterval, ts.conf.RandomizationFactorForRefreshInterval)
	defer ticker.Stop()

	handleExpiry := func(expiry time.Time) <-chan time.Time {
		if ts.conf.MarginBeforeExpiry != 0 && !expiry.IsZero() {
			delta := time.Duration(ts.conf.RandomizationFactorForMarginBeforeExpiry * float64(ts.conf.MarginBeforeExpiry))
			// [-1.0,1.0)
			plusMinus1 := 2 * (rand.Float64() - 0.5)
			jitter := time.Duration(plusMinus1 * float64(delta))
			targetTime := expiry.Add(-ts.conf.MarginBeforeExpiry + jitter)
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

func tickerWithJitter(d time.Duration, randomizationFactor float64) *backoff.Ticker {
	// (Implementation detail) It use backoff package to reduce dependency.
	backoffForJitteredTicker := &backoff.ExponentialBackOff{
		InitialInterval:     d,
		RandomizationFactor: randomizationFactor,
		Multiplier:          1.0,
		MaxInterval:         d,
		MaxElapsedTime:      0,
		Clock:               backoff.SystemClock,
	}
	backoffForJitteredTicker.Reset()

	return backoff.NewTicker(backoffForJitteredTicker)
}
