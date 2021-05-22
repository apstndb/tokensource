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

func SmartIDTokenSource(ctx context.Context, audience string) (oauth2.TokenSource, error) {
	if impSaVal := os.Getenv(impSaEnvName); impSaVal != "" {
		targetPrincipal, delegates := ParseDelegateChain(impSaVal)
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

// ParseDelegateChain split impersonate target principal and delegate chain.
// s must be non-empty string.
func ParseDelegateChain(s string) (targetPrincipal string, delegates []string) {
	if s == "" {
		panic("ParseDelegateChain: empty argument")
	}
	ss := strings.Split(s, ",")
	return ss[len(ss)-1], ss[:len(ss)-1]
}

func SmartAccessTokenSource(ctx context.Context, scopes ...string) (oauth2.TokenSource, error) {
	if impSaVal := os.Getenv(impSaEnvName); impSaVal != "" {
		targetPrincipal, delegates := ParseDelegateChain(impSaVal)
		return impersonate.CredentialsTokenSource(ctx, impersonate.CredentialsConfig{
			TargetPrincipal: targetPrincipal,
			Delegates:       delegates,
			Scopes:          scopes,
		})
	}
	return google.DefaultTokenSource(ctx, scopes...)
}
