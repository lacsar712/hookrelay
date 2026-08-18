# HookRelay

Signed ingress + reliable outbound webhook delivery (retry, circuit breaker, rate limit, DLQ, replay).

Full design: [PROJECT.md](PROJECT.md).

## Run

```text
set GOTOOLCHAIN=local
go test ./...
go run ./cmd/hookrelay
```

Open http://127.0.0.1:8080/

Default ingest secret: `dev-ingest-secret`. A loopback destination is seeded so a test event can succeed without an external URL.
