## tokensource

This repository contains some `oauth2.TokenSource`.

### `AsyncRefreshingTokenSource`

This TokenSource refreshes the token asynchronously to avoid blocking.

refs: https://qiita.com/kazegusuri/items/b6123f9d3e0777d0750c#reusetokensource%E3%81%AF%E3%83%96%E3%83%AD%E3%83%83%E3%82%AF%E3%81%99%E3%82%8B

### `SmartIDTokenSource` `SmartIDTokenSource`

They perform [ADC](https://google.aip.dev/auth/4110).
Additionally, they perform impersonation when `CLOUDSDK_AUTH_IMPERSONATE_SERVICE_ACCOUNT` is set.
