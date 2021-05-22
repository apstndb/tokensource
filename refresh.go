package tokensource

import (
	"context"
	"log"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

type ForceRefreshTokenSource struct {
	genFunc func(ctx context.Context) (oauth2.TokenSource, error)
	tokenSource oauth2.TokenSource
	refreshInterval time.Duration
	mu sync.RWMutex
}

func (ts *ForceRefreshTokenSource) Token() (*oauth2.Token, error) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.tokenSource.Token()
}

func NewForceRefreshTokenSource(ctx context.Context, refreshInterval time.Duration, genFunc func(ctx context.Context) (oauth2.TokenSource, error)) (*ForceRefreshTokenSource, error){
	b := &ForceRefreshTokenSource{genFunc: genFunc, refreshInterval: refreshInterval}
	b.flip(ctx)
	return b, nil
}

func(ts *ForceRefreshTokenSource) flip(ctx context.Context) {
	tokenSource, err := ts.genFunc(ctx)
	if err != nil {
		log.Fatalln(err)
	}
	_, err = tokenSource.Token()
	if err != nil {
		log.Fatalln(err)
	}

	ts.mu.Lock()
	ts.tokenSource = tokenSource
	ts.mu.Unlock()
}

func(ts *ForceRefreshTokenSource) Run(ctx context.Context) {
	ticker := time.NewTicker(ts.refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		ts.flip(ctx)
	}
}
