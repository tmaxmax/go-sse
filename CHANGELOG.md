# Changelog

This file tracks changes to this project. It follows the [Keep a Changelog format](https://keepachangelog.com/en/1.0.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased

### Added

- UpgradedRequest now has a flag that indicates whether a write error has occurred. This removes the need of the SendError type.

### Changed

- Server does not set any other headers besides `Content-Type`.
- UpgradedRequest does not return a SendError anymore when Write errors.

### Fixed

- Client's default response validator now ignores `Content-Type` parameters when checking if the response's content type is `text/event-stream`.

## [0.3.0] - 2021-09-18

### Added

- ReplayProviderWithGC interface, which must be satisfied by replay providers that must be cleaned up periodically.

### Changed

- Subscriptions now take a callback function instead of a channel.
- Server response headers are now sent on the first Send call, not when Upgrade is called.
- Providers are not required to add the default topic anymore. Callers of Subscribe should ensure at least a topic is specified.
- Providers' Subscribe method now blocks until the subscriber is removed.
- Server's Subscribe method automatically adds the default topic if no topic is specified.
- ReplayProvider does not require for GC to be implemented.
- Client connections take callback functions instead of channels as event listeners.
- Client connections' Unsubscribe methods are replaced by functions returned by their Subscribe counterparts.

### Fixed

- Fix replay providers not replaying the oldest message if the ID provided is of the one before that one.
- Fix replay providers hanging the caller's goroutine when a write error occurs using the default ServeHTTP implementation.
- Fix providers hanging when a write error occurs using the default ServeHTTP implementation.

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

[0.3.0]: https://github.com/tmaxmax/go-sse/releases/tag/v0.3.0
[0.2.0]: https://github.com/tmaxmax/go-sse/releases/tag/v0.2.0
[0.1.0]: https://github.com/tmaxmax/go-sse/releases/tag/v0.1.0
