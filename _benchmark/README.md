# Concurrent Benchmark

Benchmark is performed with AMD Ryzen 7 5800H on Windows 10 (and memory benchmarks additionally on WSL 2) and Go 1.18.2.

You can run it on your machine with:
```
go test -bench=. -count=10 -timeout=100m bench_test.go > report.txt
```
and then aggregate result with `benchstat`:
```
go run golang.org/x/perf/cmd/benchstat report.txt
```

## Baseline performance vs contextualized `ReadWriter` vs contextualized `Failover`

1000000 items with 16 goroutines and 10% writes.

```
                 Baseline         ReadWriter        Failover
ShardedMap       27.1ns ± 1%      33.5ns ± 1%       85.1ns ± 0% 
SyncMap          98.9ns ±13%      111ns ±18%        183ns ± 1%
ShardedMapOf     24.0ns ± 2%      32.1ns ± 1%       86.5ns ± 1%
```

## Baseline Multiple Threads With Partial Writes

Reading items from a cache with 10000 items using 16 goroutines and invoking additional writes for a fraction of reads.

```                   
                time/op (0%)    (0.1% writes)   (1% writes)     (10% writes)
sync.Map        8.02ns ± 4%     10.0ns ± 3%     17.3ns ± 2%     77.5ns ± 1% 
shardedMap      10.0ns ± 0%     11.7ns ± 1%     12.7ns ± 1%     19.5ns ± 1% 
mutexMap         110ns ± 0%      112ns ± 1%      115ns ± 1%      137ns ± 0% 
rwMutexMap      32.3ns ± 1%     47.8ns ± 0%     92.9ns ± 1%      270ns ± 1% 
shardedMapOf    10.0ns ± 2%     13.6ns ± 1%     14.4ns ± 0%     21.3ns ± 7% 
ristretto       18.4ns ± 2%     23.8ns ± 2%     27.9ns ± 2%     98.0ns ± 1% 
xsync.Map       6.70ns ± 1%     7.63ns ± 1%     8.16ns ± 7%     11.4ns ± 4% 
patrickmn       33.7ns ± 1%     47.9ns ± 0%     91.3ns ± 0%      268ns ± 1% 
bigcache        31.0ns ± 2%     34.7ns ± 0%     35.8ns ± 2%     39.9ns ± 7% 
freecache       32.7ns ± 3%     34.8ns ± 1%     35.6ns ± 2%          failed
fastcache       27.1ns ± 3%     28.6ns ± 1%     28.9ns ± 0%     37.7ns ±20%
```

## Baseline Multiple Threads Memory Usage

1000000 items with 16 goroutines and 10% writes.
Byte caches (`bigcache`, `freecache`, `fastcaches`) are used with binary encoding/decoding of a structure, which puts 
them at a minor disadvantage for extra serialization work.

```
                  time/op          MB/inuse (windows)   MB/inuse (linux)  
sync.Map          98.9ns ±13%      215 ± 0%             192 ± 0%  
shardedMap        27.1ns ± 1%      206 ± 0%             196 ± 0%  
mutexMap           206ns ± 1%      182 ± 0%             182 ± 0%  
rwMutexMap         277ns ± 1%      182 ± 0%             182 ± 0%  
shardedMapOf      24.0ns ± 2%      191 ± 0%             180 ± 0%  
ristretto          102ns ± 0%      215 ± 0%             358 ± 0%  
xsync.Map         18.6ns ± 2%      351 ± 0%             380 ± 0%  
patrickmn          283ns ± 0%      154 ± 0%             181 ± 0%  
bigcache          48.0ns ± 9%      339 ± 0%             338 ± 0%  
freecache              failed        failed             333 ± 0%  
fastcache         38.0ns ±10%      122 ± 0%            45.1 ± 0% 
```

## Baseline Single Thread Read Only

Reading items from a cache with 10000 items using single goroutine.

```                   
                time/op      
sync.Map        66.1ns ± 1%  
shardedMap      72.9ns ± 1%  
mutexMap        63.5ns ± 0%  
rwMutexMap      63.0ns ± 1%  
shardedMapOf    70.8ns ± 0%  
ristretto        133ns ± 1%  
xsync.Map       59.1ns ± 1%  
patrickmn       68.2ns ± 0%  
bigcache         211ns ± 1%  
freecache        195ns ± 2%  
fastcache        172ns ± 1%  
```
