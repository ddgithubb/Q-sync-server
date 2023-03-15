package store

import (
	"time"

	cmap "github.com/orcaman/concurrent-map/v2"
)

const (
	DEFAULT_CLEAN_INTERVAL time.Duration = 10 * time.Minute
)

type TTLCacheObject interface {
	Expires() time.Time
}

func CreateTTLCache[v TTLCacheObject]() *cmap.ConcurrentMap[string, v] {
	cache := cmap.New[v]()
	startCleaner(&cache, DEFAULT_CLEAN_INTERVAL)
	return &cache
}

func CreateTTLCacheWithInterval[v TTLCacheObject](interval time.Duration) *cmap.ConcurrentMap[string, v] {
	cache := cmap.New[v]()
	startCleaner(&cache, interval)
	return &cache
}

func startCleaner[v TTLCacheObject](cache *cmap.ConcurrentMap[string, v], interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			<-ticker.C
			now := time.Now()
			iter := cache.IterBuffered()
			for entry := range iter {
				if now.After(entry.Val.Expires()) {
					cache.Remove(entry.Key)
				}
			}
		}
	}()
}
