package admission

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type Result struct {
	Accepted  bool
	Duplicate bool
	StreamID  string
}

type RedisAdmitter struct {
	client *redis.Client
	stream string
	ttl    time.Duration
}

// The Lua script makes duplicate detection and stream insertion one atomic Redis
// operation. A separate SETNX followed by XADD creates a crash window where an event
// could be marked seen without being queued. One server-side script also removes an
// extra network round trip from the request path.
const admitScript = `
local key = KEYS[1]
local stream = KEYS[2]
local ttl = tonumber(ARGV[1])
local payload = ARGV[2]
local event_id = ARGV[3]
if redis.call('EXISTS', key) == 1 then
  return {0, ''}
end
redis.call('SET', key, '1', 'EX', ttl)
local id = redis.call('XADD', stream, '*', 'event_id', event_id, 'payload', payload)
return {1, id}
`

func New(client *redis.Client, stream string, ttl time.Duration) *RedisAdmitter {
	return &RedisAdmitter{client: client, stream: stream, ttl: ttl}
}

// Admit returns Duplicate without re-enqueuing a previously accepted event. Shared
// Redis state makes this property hold across gateway restarts and replicas. The TTL
// bounds memory use; production retention must match the payment provider's maximum
// replay window and business reconciliation requirements.
func (a *RedisAdmitter) Admit(ctx context.Context, eventID string, payload []byte) (Result, error) {
	key := "pulsegate:idempotency:" + eventID
	raw, err := a.client.Eval(ctx, admitScript, []string{key, a.stream}, int(a.ttl.Seconds()), string(payload), eventID).Result()
	if err != nil {
		return Result{}, fmt.Errorf("redis admit: %w", err)
	}
	values, ok := raw.([]interface{})
	if !ok || len(values) != 2 {
		return Result{}, fmt.Errorf("unexpected redis script response")
	}
	accepted, err := strconv.Atoi(fmt.Sprint(values[0]))
	if err != nil {
		return Result{}, fmt.Errorf("parse admit response: %w", err)
	}
	if accepted == 0 {
		return Result{Duplicate: true}, nil
	}
	return Result{Accepted: true, StreamID: fmt.Sprint(values[1])}, nil
}
