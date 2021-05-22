package main

import (
	"context"
	"encoding/json"
	"flag"
	"net/http/httputil"
	"os"

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
	printTokenFlag := flag.Bool("print-token", false, "")
	flag.Parse()

	url := flag.Arg(0)

	ctx := context.Background()

	var tokenSource oauth2.TokenSource
	if *aud != "" {
		ts, err := tokensource.SmartIDTokenSource(ctx, *aud)
		if err != nil {
			return err
		}
		tokenSource = ts
	} else {
		ts, err := tokensource.SmartAccessTokenSource(ctx, cloudPlatformScope)
		if err != nil {
			return err
		}
		tokenSource = ts
	}

	if *printTokenFlag {
		t, err := tokenSource.Token()
		if err != nil {
			return err
		}
		b, err := json.Marshal(t)
		if err != nil {
			return err
		}
		os.Stdout.Write(b)
		return nil
	}
	client := oauth2.NewClient(ctx, tokenSource)

	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	b, err := httputil.DumpResponse(resp, true)
	if err != nil {
		return err
	}
	os.Stdout.Write(b)
	return nil
}
