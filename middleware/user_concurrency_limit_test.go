package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUserConcurrencyLimit(t *testing.T) {
	const limitedUser, freeUser = 7, 8
	for _, withRedis := range []bool{false, true} {
		t.Run(fmt.Sprintf("redis=%t", withRedis), func(t *testing.T) {
			previousDB, previousRedis, previousClient := model.DB, common.RedisEnabled, common.RDB
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&model.UserLimitPolicy{}, &model.UserUsageWindow{}))
			model.DB = db
			common.RedisEnabled = withRedis
			if withRedis {
				server := miniredis.RunT(t)
				common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
			}
			t.Cleanup(func() {
				model.InvalidateUserLimitCache(limitedUser)
				model.DB, common.RedisEnabled, common.RDB = previousDB, previousRedis, previousClient
			})
			require.NoError(t, model.SaveUserLimitPolicy(limitedUser, 1, nil))

			entered, release := make(chan struct{}), make(chan struct{})
			engine := gin.New()
			engine.Use(func(c *gin.Context) {
				id, _ := strconv.Atoi(c.GetHeader("X-User"))
				c.Set("id", id)
			})
			handler := func(c *gin.Context) {
				if c.Query("hold") != "" {
					entered <- struct{}{}
					<-release
				}
				c.Status(http.StatusOK)
			}
			engine.POST("/v1/chat/completions", UserConcurrencyLimit(), handler)
			engine.POST("/mj/submit/imagine", MidjourneyUserConcurrencyLimit(), handler)
			send := func(path string, user int) *httptest.ResponseRecorder {
				recorder := httptest.NewRecorder()
				request := httptest.NewRequest(http.MethodPost, path, nil)
				request.Header.Set("X-User", strconv.Itoa(user))
				engine.ServeHTTP(recorder, request)
				return recorder
			}

			held := make(chan *httptest.ResponseRecorder)
			go func() { held <- send("/v1/chat/completions?hold=1", limitedUser) }()
			<-entered
			inFlight, err := service.UserConcurrencyInFlight(limitedUser)
			require.NoError(t, err)
			assert.Equal(t, 1, inFlight)

			rejected := send("/v1/chat/completions", limitedUser)
			assert.Equal(t, http.StatusTooManyRequests, rejected.Code)
			assert.Equal(t, "5", rejected.Header().Get("Retry-After"))
			assert.Contains(t, rejected.Body.String(), `"code":"user_concurrency_limit_reached"`)
			assert.Contains(t, rejected.Body.String(), "Concurrency limit reached. Please reduce concurrency and retry later.")
			midjourney := send("/mj/submit/imagine", limitedUser)
			assert.Equal(t, http.StatusTooManyRequests, midjourney.Code)
			assert.Contains(t, midjourney.Body.String(), `"code":429`)
			assert.Equal(t, http.StatusOK, send("/v1/chat/completions", freeUser).Code, "users without a limit are not counted")

			close(release)
			assert.Equal(t, http.StatusOK, (<-held).Code)
			inFlight, err = service.UserConcurrencyInFlight(limitedUser)
			require.NoError(t, err)
			assert.Zero(t, inFlight, "the slot is released when the request ends")
			assert.Equal(t, http.StatusOK, send("/v1/chat/completions", limitedUser).Code)

			if withRedis {
				// A lease its instance stopped renewing no longer counts.
				stale := float64(time.Now().Add(-time.Second).UnixMilli())
				require.NoError(t, common.RDB.ZAdd(context.Background(), "user_concurrency:7", &redis.Z{Score: stale, Member: "stopped-instance"}).Err())
				assert.Equal(t, http.StatusOK, send("/v1/chat/completions", limitedUser).Code)
			}
		})
	}
}
