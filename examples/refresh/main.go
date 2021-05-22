package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/apstndb/tokensource"

	"golang.org/x/oauth2"
)

const cloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"

func main() {
	if err := _main(); err != nil {
		panic(err)
	}
}

func _main() error {
	aud := flag.String("audience", "", "")
	flag.Parse()

	ctx := context.Background()

	var generatorFunc func(context.Context) (oauth2.TokenSource, error)
	if *aud != "" {
		generatorFunc = func(ctx context.Context) (oauth2.TokenSource, error) {
			return tokensource.SmartIDTokenSource(ctx, *aud)
		}
	} else {
		generatorFunc = func(ctx context.Context) (oauth2.TokenSource, error) {
			return tokensource.SmartAccessTokenSource(ctx, cloudPlatformScope)
		}
	}
	tokenSource, err := tokensource.NewForceRefreshTokenSource(ctx, tokensource.ForceRefreshConfig{RefreshInterval: 30 * time.Second}, generatorFunc)
	if err != nil {
		return err
	}

	ticker := time.NewTicker(10 * time.Second)
	for {
		begin := time.Now()
		t, err := tokenSource.Token()
		if err != nil {
			return err
		}
		end := time.Now()
		log.Printf("latency: %v, expiry: %v", end.Sub(begin), t.Expiry.Format(time.RFC3339Nano))
		<-ticker.C
	}
}
