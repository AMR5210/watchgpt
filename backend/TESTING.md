# Testing

This repo has three test layers:

1. Fast unit and handler tests that run without external services.
2. Redis integration tests behind the `integration` build tag.
3. A reproducible load-test script for latency and throughput numbers.

## Unit tests

```bash
GOCACHE=/tmp/watchgpt-gocache go test ./...
```

Current result from local run:

```text
ok github.com/AMR5210/watchgpt/backend/internal/cache
ok github.com/AMR5210/watchgpt/backend/internal/handler
ok github.com/AMR5210/watchgpt/backend/internal/middleware
```

Coverage command:

```bash
GOCACHE=/tmp/watchgpt-gocache go test -coverpkg=./internal/... -coverprofile=/tmp/watchgpt-cover.out ./...
GOCACHE=/tmp/watchgpt-gocache go tool cover -func=/tmp/watchgpt-cover.out
```

Current coverage highlights:

```text
internal/cache HashKey:          100.0%
internal/middleware Auth:        100.0%
internal/requestctx helpers:    100.0%
internal/handler package:        79.5%
internal/middleware Cognito JWT:  69.2%
```

Handler coverage is at 79.5% because some edge cases around malformed multipart uploads and specific OpenAI error shapes are not yet covered. Cognito JWT verification is at 69.2% because certain failure paths (JWKS endpoint unreachable, key rotation mid-request, malformed token headers) require mocking external Cognito infrastructure.

## Redis integration tests

Start Redis:

```bash
docker compose -f docker-compose.test.yml up -d redis
```

Run integration tests:

```bash
GOCACHE=/tmp/watchgpt-gocache REDIS_ADDR=127.0.0.1:6379 go test -tags=integration ./...
```

Stop Redis:

```bash
docker compose -f docker-compose.test.yml down
```

The integration test verifies that the Redis-backed sliding-window limiter returns `429` after the configured limit.

## Load test

The load-test wrapper defaults to an in-process `/health` benchmark:

```bash
./scripts/load-test.sh
```

To test a real running server:

```bash
URL=http://127.0.0.1:18080/health REQUESTS=10000 CONCURRENCY=50 ./scripts/load-test.sh
```

For authenticated JSON endpoints:

```bash
URL=https://api.example.com/api/v1/chat \
METHOD=POST \
TOKEN="$TOKEN" \
BODY='{"messages":[{"role":"user","content":"hello"}]}' \
REQUESTS=1000 \
CONCURRENCY=20 \
./scripts/load-test.sh
```

## Latest local numbers

Machine-local HTTP `/health` run:

```text
requests: 10000
concurrency: 50
elapsed: 0.193s
throughput: 51857.9 req/s
errors: 0
statuses: map[200:10000]
min: 39.791µs
p50: 822.042µs
p95: 2.165667ms
p99: 3.271792ms
max: 6.908959ms
```

In-process `/health` baseline:

```text
requests: 10000
concurrency: 50
elapsed: 0.020s
throughput: 501661.8 req/s
errors: 0
statuses: map[200:10000]
min: 1.084µs
p50: 3.708µs
p95: 113.667µs
p99: 3.555292ms
max: 11.579708ms
```
