package main

import (
	"context"
	"flag"
	"fmt"
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

	tokenSource, err := tokensource.NewForceRefreshTokenSource(ctx, 30 * time.Second, func(ctx context.Context) (oauth2.TokenSource, error) {
		if *aud != "" {
			return tokensource.SmartIDTokenSource(ctx, *aud)
		} else {
			return tokensource.SmartAccessTokenSource(ctx, cloudPlatformScope)
		}
	})
	if err != nil {
		return err
	}
	go tokenSource.Run(ctx)

	ticker := time.NewTicker(10 * time.Second)
	for {
		<- ticker.C
		begin := time.Now()
		t, err := tokenSource.Token()
		if err != nil {
			return err
		}
		end := time.Now()
		fmt.Printf("latency: %v, expiry: %v\n", end.Sub(begin), t.Expiry.Format(time.RFC3339Nano))
	}
}
