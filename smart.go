package tokensource

import (
	"context"
	"os"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/idtoken"
	"google.golang.org/api/impersonate"
)

const impSaEnvName = "CLOUDSDK_AUTH_IMPERSONATE_SERVICE_ACCOUNT"

// SmartIDTokenSource generate oauth2.TokenSource which generates ID token and supports CLOUDSDK_AUTH_IMPERSONATE_SERVICE_ACCOUNT environment variable.
func SmartIDTokenSource(ctx context.Context, audience string) (oauth2.TokenSource, error) {
	if impSaVal := os.Getenv(impSaEnvName); impSaVal != "" {
		targetPrincipal, delegates := parseDelegateChain(impSaVal)
		idCfg := impersonate.IDTokenConfig{
			Audience:        audience,
			TargetPrincipal: targetPrincipal,
			Delegates:       delegates,
			// Cloud IAP requires email claim.
			IncludeEmail: true,
		}
		return impersonate.IDTokenSource(ctx, idCfg)
	}

	return idtoken.NewTokenSource(ctx, audience)
}

// parseDelegateChain split impersonate target principal and delegate chain.
// s must be non-empty string.
func parseDelegateChain(s string) (targetPrincipal string, delegates []string) {
	if s == "" {
		panic("parseDelegateChain: empty argument")
	}
	ss := strings.Split(s, ",")
	return ss[len(ss)-1], ss[:len(ss)-1]
}

// SmartAccessTokenSource generate oauth2.TokenSource which generates access token and supports CLOUDSDK_AUTH_IMPERSONATE_SERVICE_ACCOUNT environment variable.
func SmartAccessTokenSource(ctx context.Context, scopes ...string) (oauth2.TokenSource, error) {
	if impSaVal := os.Getenv(impSaEnvName); impSaVal != "" {
		targetPrincipal, delegates := parseDelegateChain(impSaVal)
		return impersonate.CredentialsTokenSource(ctx, impersonate.CredentialsConfig{
			TargetPrincipal: targetPrincipal,
			Delegates:       delegates,
			Scopes:          scopes,
		})
	}
	return google.DefaultTokenSource(ctx, scopes...)
}
