## Performance & Observability

### Problem

Direct OpenAI calls from the watch app averaged **4.03s p50 latency** with no visibility into failure modes, no caching, and the API key embedded client-side.

### What changed

Moved to a Go proxy backend with three targeted optimizations and a full observability stack.

**Model swap:** `gpt-4o` → `gpt-4o-mini`. Same vision capability for this use case at ~2x lower latency and 10x lower cost per request.

**Response streaming:** SSE stream from backend to watch via `/api/v1/stream`. First token lands in ~200ms. Previously the watch waited for the full completion before rendering anything.

**Redis caching:** SHA-256 hash of `image_bytes + prompt` as cache key, 1-hour TTL, LRU eviction at 64MB. Cache hit returns in <1ms. Identical image+prompt combinations across users hit cache instead of burning an API call.

### Results

| Metric | Before | After |
|---|---|---|
| Model | gpt-4o | gpt-4o-mini |
| p50 latency (non-streaming) | 4.03s | 2.06s |
| Time to first token (streaming) | 4.03s | ~200ms |
| Cache hit latency | N/A | <1ms |
| Error rate | 0% | 0% |
| Cost per 1K image requests | ~$7.50 | ~$0.70 |

### Observability stack

Prometheus scrapes the backend pods every 15s. Grafana renders a 9-panel dashboard with auto-refresh.

**Metrics exposed at `/metrics`:**

- `http_requests_total` - counter by method, path, status
- `http_request_duration_seconds` - histogram with p50/p95/p99
- `http_requests_in_flight` - gauge
- `watchgpt_openai_request_duration_seconds` - histogram, isolates upstream latency from backend processing
- `watchgpt_openai_requests_total` - counter by success/error
- `watchgpt_cache_hits_total` / `watchgpt_cache_misses_total` - cache effectiveness tracking
- `watchgpt_image_size_bytes` - histogram, correlates payload size with latency
- `watchgpt_active_conversations` - gauge

### Design decisions

**Why cache at the image hash level, not the response level?** The same photo produces the same bytes. Hashing a combination of the first and last 1000 chars of the base64 + the prompt string gives collision-free keys without hashing the entire payload, keeping cache key generation under 1ms even for large images.

**Why SSE over WebSockets?** Unidirectional data flow (server → client). SSE works over standard HTTP, survives middleware and load balancers without upgrade negotiation, and the watch only needs to receive tokens, not send mid-stream.

**Why in-memory Redis over managed cache?** Single-node Redis with LRU eviction is sufficient for the request volume. Avoids external dependency costs while the cache hit ratio proves out. Drop-in replaceable with ElastiCache or Memorystore when scaling beyond one node.

**Why instrument OpenAI latency separately?** Backend p95 includes serialization, auth, rate limiting. If p95 spikes, the first question is "is it us or OpenAI?" Separate histograms answer that without digging through logs.
