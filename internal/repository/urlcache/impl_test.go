package urlcache_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"

	"github.com/rjchien728/short-url/internal/domain/entity"
	"github.com/rjchien728/short-url/internal/repository/urlcache"
)

type URLCacheSuite struct {
	suite.Suite
	mr    *miniredis.Miniredis
	cache *urlcache.Cache
}

func (s *URLCacheSuite) SetupSuite() {
	mr, err := miniredis.Run()
	s.Require().NoError(err)
	s.mr = mr

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	s.cache = urlcache.NewCache(rdb)
}

func (s *URLCacheSuite) TearDownSuite() {
	s.mr.Close()
}

func (s *URLCacheSuite) SetupTest() {
	// Flush all keys before each test.
	s.mr.FlushAll()
}

// --- Test cases ---

func (s *URLCacheSuite) TestSet_Get_HappyPath() {
	ctx := context.Background()
	expires := time.Now().Add(24 * time.Hour)

	url := &entity.ShortURL{
		ID:        1001,
		ShortCode: "abc1234567",
		LongURL:   "https://example.com",
		CreatorID: "user_01",
		ExpiresAt: &expires,
		CreatedAt: time.Now(),
	}

	err := s.cache.Set(ctx, url.ShortCode, url, urlcache.CacheTTL)
	s.Require().NoError(err)

	got, err := s.cache.Get(ctx, url.ShortCode)
	s.Require().NoError(err)
	s.Require().NotNil(got)
	s.Equal(url.ID, got.ID)
	s.Equal(url.ShortCode, got.ShortCode)
	s.Equal(url.LongURL, got.LongURL)
	s.Equal(url.CreatorID, got.CreatorID)
}

func (s *URLCacheSuite) TestGet_CacheMiss_ReturnsNilNil() {
	ctx := context.Background()

	got, err := s.cache.Get(ctx, "notexists0")
	s.Require().NoError(err) // must not be an error
	s.Nil(got)
}

func (s *URLCacheSuite) TestDelete_RemovesKey() {
	ctx := context.Background()

	url := &entity.ShortURL{
		ID:        2001,
		ShortCode: "del0000001",
		LongURL:   "https://example.com/del",
		CreatorID: "user_02",
		CreatedAt: time.Now(),
	}

	s.Require().NoError(s.cache.Set(ctx, url.ShortCode, url, urlcache.CacheTTL))

	// Confirm it exists.
	got, err := s.cache.Get(ctx, url.ShortCode)
	s.Require().NoError(err)
	s.NotNil(got)

	// Delete and confirm miss.
	s.Require().NoError(s.cache.Delete(ctx, url.ShortCode))

	got, err = s.cache.Get(ctx, url.ShortCode)
	s.Require().NoError(err)
	s.Nil(got)
}

func (s *URLCacheSuite) TestSet_IncludesOGMetadata() {
	ctx := context.Background()

	url := &entity.ShortURL{
		ID:        3001,
		ShortCode: "ogcache001",
		LongURL:   "https://example.com/og",
		CreatorID: "user_03",
		CreatedAt: time.Now(),
		OGMetadata: &entity.OGMetadata{
			Title:    "My Title",
			SiteName: "My Site",
		},
	}

	s.Require().NoError(s.cache.Set(ctx, url.ShortCode, url, urlcache.CacheTTL))

	got, err := s.cache.Get(ctx, url.ShortCode)
	s.Require().NoError(err)
	s.Require().NotNil(got)
	s.Require().NotNil(got.OGMetadata)
	s.Equal("My Title", got.OGMetadata.Title)
	s.Equal("My Site", got.OGMetadata.SiteName)
}

func (s *URLCacheSuite) TestGet_SlidingTTL_RefreshedOnHit() {
	ctx := context.Background()

	url := &entity.ShortURL{
		ID:        4001,
		ShortCode: "sliding001",
		LongURL:   "https://example.com/sliding",
		CreatedAt: time.Now(),
	}

	s.Require().NoError(s.cache.Set(ctx, url.ShortCode, url, urlcache.CacheTTL))

	// Advance time by half the TTL.
	s.mr.FastForward(urlcache.CacheTTL / 2)

	// A Get should refresh the TTL.
	got, err := s.cache.Get(ctx, url.ShortCode)
	s.Require().NoError(err)
	s.Require().NotNil(got)

	// After another full TTL from when we started, the key should still be alive
	// because the hit reset the TTL.
	s.mr.FastForward(urlcache.CacheTTL / 2)

	got, err = s.cache.Get(ctx, url.ShortCode)
	s.Require().NoError(err)
	s.Require().NotNil(got, "key should still be alive after sliding TTL refresh")
}

func (s *URLCacheSuite) TestSetNotFound_Get_ReturnsErrNotFound() {
	ctx := context.Background()
	shortCode := "ghost00001"

	s.Require().NoError(s.cache.SetNotFound(ctx, shortCode))

	got, err := s.cache.Get(ctx, shortCode)
	s.Require().Error(err)
	s.True(errors.Is(err, entity.ErrNotFound), "expected ErrNotFound for negative cache hit")
	s.Nil(got)
}

func TestURLCache(t *testing.T) {
	suite.Run(t, new(URLCacheSuite))
}
