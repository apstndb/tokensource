package tokensource

import (
	"context"
	"log"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"golang.org/x/oauth2"
)

const defaultInterval = 30 * time.Minute

type AsyncRefreshingTokenSource struct {
	genFunc func(ctx context.Context) (oauth2.TokenSource, error)
	token   *oauth2.Token
	conf    AsyncRefreshingConfig
	mu      sync.Mutex
	// ctx is stored because genFunc use context.Context but TokenSource.Token() doesn't take context.Context.
	ctx context.Context
}

func (ts *AsyncRefreshingTokenSource) Token() (*oauth2.Token, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.token.Valid() {
		return ts.token, nil
	}
	tokenSource, err := ts.genFunc(ts.ctx)
	if err != nil {
		return nil, err
	}
	token, err := tokenSource.Token()
	// Don't perform exponential backoff because this method doesn't take current context.
	if err != nil {
		return nil, err
	}
	ts.token = token
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
	// If not set, 30 minutes is default interval.
	RefreshInterval time.Duration
	// RandomizationFactorForRefreshInterval is randomization factor for RefreshInterval.
	RandomizationFactorForRefreshInterval float64

	// Backoff is backoff configuration for TokenSource.Token().
	// If not set, backoff.NewExponentialBackOff is used as the default value.
	// See also https://pkg.go.dev/github.com/cenkalti/backoff/v4#NewExponentialBackOff.
	// If IsRetryable isn't set, no backoff will be performed.
	Backoff backoff.BackOff

	// IsRetryable is the predicate function for retryable errors.
	// Default: never retry.
	IsRetryable func(err error) bool
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
			if os.Getenv("DEBUG") != "" {
				log.Printf("AsyncRefreshingTokenSource.flip() error: %v", err)
			}
			if ts.conf.IsRetryable == nil || !ts.conf.IsRetryable(err) {
				return backoff.Permanent(err)
			}
			return err
		}
		token = t
		return nil
	}, ts.conf.Backoff)

	ts.mu.Lock()
	ts.token = token
	ts.mu.Unlock()

	if err != nil {
		return time.Time{}, err
	}
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
			log.Println("AsyncRefreshingTokenSource encounter unresolved error:", err)
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
