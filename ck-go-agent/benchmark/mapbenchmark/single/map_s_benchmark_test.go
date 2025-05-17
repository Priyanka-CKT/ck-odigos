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
	numRequests = 5000000 // Total number of requests to simulate
	numKeys     = 500   // Total number of unique keys
)

// Benchmark with a regular map and mutex
type MutexMap struct {
	mu sync.Mutex
	m  map[string]*atomic.Int64
}

func (m *MutexMap) Increment(key string) {
	m.mu.Lock()
	if _, ok := m.m[key]; !ok {
		m.m[key] = new(atomic.Int64)
	}
	m.m[key].Add(1)
	m.mu.Unlock()
}

// Benchmark with sync.Map
type SyncMap struct {
	m sync.Map
}

func (m *SyncMap) Increment(key string) {
	value, _ := m.m.LoadOrStore(key, new(atomic.Int64))
	value.(*atomic.Int64).Add(1)
}

// Benchmark with tidwall/shardmap
type ShardMap struct {
	m *shardmap.Map
}

func (m *ShardMap) Increment(key string) {
	m.m.Set(key, func(value interface{}, exists bool) interface{} {
		if !exists {
			return int64(1)
		}
		return value.(int64) + 1
	})
}

// Benchmarking function
func BenchmarkMaps(b *testing.B) {
	keys := generateKeys(numKeys)
	b.Run("MutexMap", func(b *testing.B) {
		mutexMap := &MutexMap{m: make(map[string]*atomic.Int64)}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			key := keys[rand.Intn(len(keys))]
			mutexMap.Increment(key)
		}
	})

	b.Run("SyncMap", func(b *testing.B) {
		syncMap := &SyncMap{}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			key := keys[rand.Intn(len(keys))]
			syncMap.Increment(key)
		}
	})

	b.Run("ShardMap", func(b *testing.B) {
		shardMap := &ShardMap{m: shardmap.New(256)} // Initialize shardmap with 256 shards
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			key := keys[rand.Intn(len(keys))]
			shardMap.Increment(key)
		}
	})
}

func generateKeys(n int) []string {
	keys := make([]string, n)
	for i := 0; i < n; i++ {
		keys[i] = "key" + strconv.Itoa(i)
	}
	return keys
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

