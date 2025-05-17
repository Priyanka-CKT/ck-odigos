package main

import (
	"math/rand"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tidwall/shardmap"
)

const (
	benchmarkNumKeys    = 500 // Total number of unique keys
	benchmarkNumWorkers = 100 // Number of concurrent goroutines
)

// Benchmark with a regular map and mutex
type BenchMutexMap struct {
	mu sync.Mutex
	m  map[string]*atomic.Int64
}

func (m *BenchMutexMap) Increment(key string) {
	m.mu.Lock()
	if _, ok := m.m[key]; !ok {
		m.m[key] = new(atomic.Int64)
	}
	m.m[key].Add(1)
	m.mu.Unlock()
}

// Benchmark with sync.Map
type BenchSyncMap struct {
	m sync.Map
}

func (m *BenchSyncMap) Increment(key string) {
	value, _ := m.m.LoadOrStore(key, new(atomic.Int64))
	value.(*atomic.Int64).Add(1)
}

// Benchmark with tidwall/shardmap
type BenchShardMap struct {
	m *shardmap.Map
}

func (m *BenchShardMap) Increment(key string) {
	m.m.Set(key, func(value interface{}, exists bool) interface{} {
		if !exists {
			return int64(1)
		}
		return value.(int64) + 1
	})
}

// Benchmarking function
func BenchmarkConcurrentMaps(b *testing.B) {
	keys := benchmarkGenerateKeys(benchmarkNumKeys)

	b.Run("MutexMap", func(b *testing.B) {
		mutexMap := &BenchMutexMap{m: make(map[string]*atomic.Int64)}
		var wg sync.WaitGroup
		requestsPerWorker := b.N / benchmarkNumWorkers

		b.ResetTimer()
		for w := 0; w < benchmarkNumWorkers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < requestsPerWorker; i++ {
					key := keys[rand.Intn(len(keys))]
					mutexMap.Increment(key)
				}
			}()
		}
		wg.Wait()
	})

	b.Run("SyncMap", func(b *testing.B) {
		syncMap := &BenchSyncMap{}
		var wg sync.WaitGroup
		requestsPerWorker := b.N / benchmarkNumWorkers

		b.ResetTimer()
		for w := 0; w < benchmarkNumWorkers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < requestsPerWorker; i++ {
					key := keys[rand.Intn(len(keys))]
					syncMap.Increment(key)
				}
			}()
		}
		wg.Wait()
	})

	b.Run("ShardMap", func(b *testing.B) {
		shardMap := &BenchShardMap{m: shardmap.New(256)} // Initialize shardmap with 256 shards
		var wg sync.WaitGroup
		requestsPerWorker := b.N / benchmarkNumWorkers

		b.ResetTimer()
		for w := 0; w < benchmarkNumWorkers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < requestsPerWorker; i++ {
					key := keys[rand.Intn(len(keys))]
					shardMap.Increment(key)
				}
			}()
		}
		wg.Wait()
	})
}

func benchmarkGenerateKeys(n int) []string {
	keys := make([]string, n)
	for i := 0; i < n; i++ {
		keys[i] = "key" + strconv.Itoa(i)
	}
	return keys
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
