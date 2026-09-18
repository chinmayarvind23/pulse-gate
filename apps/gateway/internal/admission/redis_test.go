package admission

import (
	"context"
	"github.com/redis/go-redis/v9"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// clientForTest isolates integration keys so unrelated Redis data remains untouched.
func clientForTest(t *testing.T) (*redis.Client, string) {
	t.Helper()
	addr := os.Getenv("HOOKGUARD_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("set HOOKGUARD_TEST_REDIS_ADDR for Redis integration")
	}
	c := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { c.Close() })
	stream := "test:" + t.Name() + ":" + time.Now().Format("150405.000000000")
	t.Cleanup(func() {
		// Incremental cleanup avoids pausing unrelated traffic in a shared test Redis.
		keys := c.Scan(context.Background(), 0, stream+"*", 100).Iterator()
		for keys.Next(context.Background()) {
			if err := c.Del(context.Background(), keys.Val()).Err(); err != nil {
				t.Error(err)
			}
		}
		if err := keys.Err(); err != nil {
			t.Error(err)
		}
	})
	return c, stream
}

// TestConcurrentDuplicates exercises the race across pooled Redis connections.
func TestConcurrentDuplicates(t *testing.T) {
	c, s := clientForTest(t)
	a := New(c, s, time.Minute)
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := a.Admit(context.Background(), "evt", []byte(`{}`))
			if e != nil {
				t.Error(e)
			}
			if r.Accepted {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	if accepted.Load() != 1 {
		t.Fatalf("accepted=%d", accepted.Load())
	}
	if n := c.XLen(context.Background(), s).Val(); n != 1 {
		t.Fatalf("stream length=%d", n)
	}
}

// TestFailedAppendCanRetry proves script errors cannot poison the replay marker.
func TestFailedAppendCanRetry(t *testing.T) {
	c, s := clientForTest(t)
	ctx := context.Background()
	a := New(c, s, time.Minute)
	c.Set(ctx, s, "wrong type", 0)
	if _, err := a.Admit(ctx, "evt", []byte(`{}`)); err == nil {
		t.Fatal("expected wrong type")
	}
	c.Del(ctx, s)
	result, err := a.Admit(ctx, "evt", []byte(`{}`))
	if err != nil || !result.Accepted {
		t.Fatalf("retry lost: %+v %v", result, err)
	}
}

// TestConflictingPayload rejects reusing an identifier for another transaction.
func TestConflictingPayload(t *testing.T) {
	c, s := clientForTest(t)
	a := New(c, s, time.Minute)
	ctx := context.Background()
	if _, err := a.Admit(ctx, "evt", []byte(`{"v":1}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Admit(ctx, "evt", []byte(`{"v":2}`)); err == nil {
		t.Fatal("conflicting payload accepted")
	}
}
