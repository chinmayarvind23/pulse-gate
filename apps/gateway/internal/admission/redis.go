package admission

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/redis/go-redis/v9"
	"time"
)

var ErrConflict = errors.New("event_id already used with a different payload")
var ErrFull = errors.New("admission queue is full")

type Result struct {
	Accepted  bool
	Duplicate bool
	StreamID  string
}
type RedisAdmitter struct {
	client   *redis.Client
	stream   string
	ttl      time.Duration
	MaxQueue int64
	script   *redis.Script
}

// Scripts isolate clients but do not roll back command errors. Remove the marker
// if append fails, so a retry can still admit the event.
const admitScript = `
local prior = redis.call('GET', KEYS[1])
if prior then
 if prior ~= ARGV[4] then return {-1,''} end
 return {0,''}
end
if redis.call('XLEN',KEYS[2]) >= tonumber(ARGV[5]) then return {-2,''} end
redis.call('SET',KEYS[1],ARGV[4],'EX',ARGV[1])
local added = redis.pcall('XADD',KEYS[2],'*','event_id',ARGV[3],'payload',ARGV[2])
if type(added)=='table' and added.err then
 redis.call('DEL',KEYS[1])
 return redis.error_reply(added.err)
end
return {1,added}
`

// New scopes replay state to the input stream and caches the Lua script hash.
func New(client *redis.Client, stream string, ttl time.Duration) *RedisAdmitter {
	return &RedisAdmitter{client: client, stream: stream, ttl: ttl, MaxQueue: 1000000, script: redis.NewScript(admitScript)}
}

// Admit binds an event identifier to canonical payload bytes for the retention window.
func (a *RedisAdmitter) Admit(ctx context.Context, eventID string, payload []byte) (Result, error) {
	digest := sha256.Sum256(payload)
	key := a.stream + ":idempotency:" + eventID
	raw, err := a.script.Run(ctx, a.client, []string{key, a.stream}, int64(a.ttl.Seconds()), payload, eventID, hex.EncodeToString(digest[:]), a.MaxQueue).Slice()
	if err != nil {
		return Result{}, fmt.Errorf("redis admit: %w", err)
	}
	if len(raw) != 2 {
		return Result{}, fmt.Errorf("unexpected Redis response")
	}
	code, ok := raw[0].(int64)
	if !ok {
		return Result{}, fmt.Errorf("unexpected Redis result type")
	}
	switch code {
	case -1:
		return Result{}, ErrConflict
	case -2:
		return Result{}, ErrFull
	case 0:
		return Result{Duplicate: true}, nil
	case 1:
		id, ok := raw[1].(string)
		if !ok {
			return Result{}, fmt.Errorf("unexpected stream id")
		}
		return Result{Accepted: true, StreamID: id}, nil
	default:
		return Result{}, fmt.Errorf("unexpected Redis result code")
	}
}
