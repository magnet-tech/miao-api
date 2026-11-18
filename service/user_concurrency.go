package service

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
)

const (
	// UserConcurrencyRetryAfter is the Retry-After, in seconds, of a request
	// turned away by the concurrency limit.
	UserConcurrencyRetryAfter = 5

	concurrencyLeaseTTL     = 90 * time.Second
	concurrencyRenewEvery   = 30 * time.Second
	concurrencyRedisTimeout = 2 * time.Second
)

// With Redis every instance shares one sorted set per user holding the leases
// of its running requests, scored by when they expire. A running request
// renews its lease, so the lease of a request on a stopped instance expires on
// its own. Without Redis there is one instance and an in-process count.
var acquireConcurrencyScript = redis.NewScript(`
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', ARGV[1])
if redis.call('ZCARD', KEYS[1]) >= tonumber(ARGV[2]) then
  return 0
end
redis.call('ZADD', KEYS[1], ARGV[3], ARGV[4])
redis.call('PEXPIRE', KEYS[1], ARGV[5])
return 1`)

var (
	concurrencyMu    sync.Mutex
	concurrencyLocal = map[int]int{}
)

func concurrencyKey(userID int) string {
	return "user_concurrency:" + strconv.Itoa(userID)
}

func concurrencyUsesRedis() bool {
	return common.RedisEnabled && common.RDB != nil
}

// ConcurrencyLease is one running request counted against its user's
// concurrency limit.
type ConcurrencyLease struct {
	userID int
	member string // the Redis lease; empty for the in-process count
	stop   context.CancelFunc
	once   sync.Once
}

// AcquireUserConcurrency takes a slot for one request of the user, or reports
// false when the user already runs limit requests.
func AcquireUserConcurrency(userID int, limit int) (*ConcurrencyLease, bool, error) {
	if !concurrencyUsesRedis() {
		concurrencyMu.Lock()
		defer concurrencyMu.Unlock()
		if concurrencyLocal[userID] >= limit {
			return nil, false, nil
		}
		concurrencyLocal[userID]++
		return &ConcurrencyLease{userID: userID}, true, nil
	}

	lease := &ConcurrencyLease{userID: userID, member: common.GetUUID()}
	now := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), concurrencyRedisTimeout)
	defer cancel()
	acquired, err := acquireConcurrencyScript.Run(ctx, common.RDB, []string{concurrencyKey(userID)},
		now.UnixMilli(), limit, now.Add(concurrencyLeaseTTL).UnixMilli(), lease.member, concurrencyLeaseTTL.Milliseconds(),
	).Int()
	if err != nil || acquired != 1 {
		return nil, false, err
	}
	renewCtx, stop := context.WithCancel(context.Background())
	lease.stop = stop
	go lease.renew(renewCtx)
	return lease, true, nil
}

func (lease *ConcurrencyLease) renew(ctx context.Context) {
	ticker := time.NewTicker(concurrencyRenewEvery)
	defer ticker.Stop()
	key := concurrencyKey(lease.userID)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		redisCtx, cancel := context.WithTimeout(ctx, concurrencyRedisTimeout)
		// ZADD XX only updates a lease that still exists, so a renewal racing
		// the release never brings the lease back.
		_, err := common.RDB.TxPipelined(redisCtx, func(pipe redis.Pipeliner) error {
			pipe.ZAddXX(redisCtx, key, &redis.Z{Score: float64(time.Now().Add(concurrencyLeaseTTL).UnixMilli()), Member: lease.member})
			pipe.PExpire(redisCtx, key, concurrencyLeaseTTL)
			return nil
		})
		cancel()
		if err != nil && ctx.Err() == nil {
			common.SysError(common.LogText("failed to renew a concurrency lease of user %d: %s", lease.userID, err.Error()))
		}
	}
}

// Release frees the slot; later calls do nothing.
func (lease *ConcurrencyLease) Release() {
	lease.once.Do(func() {
		if lease.member == "" {
			concurrencyMu.Lock()
			defer concurrencyMu.Unlock()
			if concurrencyLocal[lease.userID]--; concurrencyLocal[lease.userID] <= 0 {
				delete(concurrencyLocal, lease.userID)
			}
			return
		}
		lease.stop()
		ctx, cancel := context.WithTimeout(context.Background(), concurrencyRedisTimeout)
		defer cancel()
		if err := common.RDB.ZRem(ctx, concurrencyKey(lease.userID), lease.member).Err(); err != nil {
			common.SysError(common.LogText("failed to release a concurrency lease of user %d: %s", lease.userID, err.Error()))
		}
	})
}

// UserConcurrencyInFlight counts the user's running requests.
func UserConcurrencyInFlight(userID int) (int, error) {
	if !concurrencyUsesRedis() {
		concurrencyMu.Lock()
		defer concurrencyMu.Unlock()
		return concurrencyLocal[userID], nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), concurrencyRedisTimeout)
	defer cancel()
	count, err := common.RDB.ZCount(ctx, concurrencyKey(userID), strconv.FormatInt(time.Now().UnixMilli(), 10), "+inf").Result()
	return int(count), err
}
