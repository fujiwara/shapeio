# Changelog

## [v1.1.0](https://github.com/fujiwara/shapeio/compare/v1.0.0...v1.1.0) - 2026-05-22
- [FIX] Change t.Errorf to t.Error When Not Using Formatting Directives by @sbward in https://github.com/fujiwara/shapeio/pull/3
- Support dynamic rate limit changes via SetRateLimit by @fujiwara in https://github.com/fujiwara/shapeio/pull/5
- Introduce Songmu/tagpr for release automation by @fujiwara in https://github.com/fujiwara/shapeio/pull/6
- Pass GITHUB_TOKEN to tagpr step by @fujiwara in https://github.com/fujiwara/shapeio/pull/7
- Loosen rate test tolerance to +/-5% and skip too-short combinations by @fujiwara in https://github.com/fujiwara/shapeio/pull/9
- Add SetRateLimitEvery for "N bytes per duration" rate expression by @fujiwara in https://github.com/fujiwara/shapeio/pull/10
- Add ReadCloser / WriteCloser to bundle Close with rate-limited io by @fujiwara in https://github.com/fujiwara/shapeio/pull/11
- Drop go-humanize dependency by inlining a tiny IBytes equivalent by @fujiwara in https://github.com/fujiwara/shapeio/pull/12
- Add _example/download CLI: curl-like downloader with SIGUSR1 rate toggle by @fujiwara in https://github.com/fujiwara/shapeio/pull/13

## [v1.0.0](https://github.com/fujiwara/shapeio/commits/v1.0.0) - 2021-02-22
- switch impl to use golang.org/x/time/rate by @fujiwara in https://github.com/fujiwara/shapeio/pull/1
