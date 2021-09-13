# Changelog

This file tracks changes to this project. It follows the [Keep a Changelog format](https://keepachangelog.com/en/1.0.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.0] - 2021-09-13

### Added

- Text/JSON marshalers and unmarshalers, and SQL scanners and valuers for the EventID type (previously event.ID).
- Check for http.NoBody before resetting the request body on client reconnect.

### Changed

- Package structure. The module is now refactored into a single package with an idiomatic name. This has resulted in various name changes:
  - `client.Error` - `sse.ConnectionError`
  - `event.Event` - `sse.Message` (previous `server.Message` is removed, see next change)
  - `event.ID` - `sse.EventID`
  - `event.NewID` - `sse.NewEventID`
  - `event.MustID` - `sse.MustEventID`
  - `server.Connection` - `sse.UpgradedRequest`
  - `server.NewConnection` - `sse.Upgrade`
  - `server.ErrUnsupported` - `sse.ErrUpgradeUnsupported`
  - `server.New` - `sse.NewServer`.
- `event.Event` is merged with `server.Message`, becoming `sse.Message`. This affects the `sse.Server.Publish` function, which doesn't take a `topic` parameter anymore.
- The server's constructor doesn't take an `Provider` as a parameter. It instead takes multiple optional `ServerOptions`. The `WithProvider` option is now used to pass custom providers to the server.
- The `ReplayProvider` interface's `Put` method now takes a `**Message` instead of a `*Message`. This change also affects the replay providers in this package: `ValidReplayProvider` and `FiniteReplayProvider`.
- The `Provider` interface's `Publish` method now takes a `*Message` instead of a `Message`. This change also affects `Joe`, the provider in this package.
- The `UpgradedRequest`'s `Send` now method takes a `*Message` as parameter.

## [0.1.0] - 2021-09-11 First release
