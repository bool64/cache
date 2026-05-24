# Concurrent Benchmark

Relative percentages in tables are shown from the best result in the same column, where `100%` is best.

## Apple M3 Max

Benchmark is performed with Apple M3 Max on `darwin/arm64` and Go 1.26.1.

You can run it on your machine with:

```bash
go test -bench=. -count=10 -timeout=100m bench_test.go > report.txt
```

and then aggregate result with `benchstat`:

```bash
go run golang.org/x/perf/cmd/benchstat report.txt
```

### Baseline performance vs contextualized `ReadWriter` vs contextualized `Failover`

1000000 items with 14 goroutines and 10% writes.

| Backend      | Baseline         | ReadWriter       | Failover        |
|--------------|------------------|------------------|-----------------|
| ShardedMap   | 42.2ns/op (438%) | 56.3ns/op (135%) | 136ns/op (102%) |
| SyncMap      | 9.64ns/op (100%) | 112ns/op (269%)  | 133ns/op (100%) |
| ShardedMapOf | 41.4ns/op (429%) | 41.6ns/op (100%) | 137ns/op (103%) |

### Baseline Multiple Threads With Partial Writes

Reading items from a cache with 10000 items using 14 goroutines and invoking additional writes for a fraction of reads.

| Backend      | 0% writes         | 0.1% writes       | 1% writes         | 10% writes        |
|--------------|-------------------|-------------------|-------------------|-------------------|
| sync.Map     | 2.07ns/op (100%)  | 2.12ns/op (100%)  | 2.40ns/op (100%)  | 4.95ns/op (105%)  |
| shardedMap   | 67.7ns/op (3271%) | 39.4ns/op (1858%) | 26.0ns/op (1083%) | 37.5ns/op (796%)  |
| mutexMap     | 145ns/op (7005%)  | 145ns/op (6840%)  | 148ns/op (6167%)  | 176ns/op (3737%)  |
| rwMutexMap   | 128ns/op (6184%)  | 93.9ns/op (4430%) | 54.3ns/op (2263%) | 56.9ns/op (1208%) |
| shardedMapOf | 71.2ns/op (3440%) | 40.5ns/op (1910%) | 25.1ns/op (1046%) | 38.4ns/op (815%)  |
| ristretto    | 23.1ns/op (1116%) | 17.3ns/op (816%)  | 24.8ns/op (1033%) | 133ns/op (2824%)  |
| xsync.Map    | 2.52ns/op (122%)  | 2.85ns/op (134%)  | 3.83ns/op (160%)  | 7.23ns/op (154%)  |
| dlht.Map     | 3.53ns/op (171%)  | 3.48ns/op (164%)  | 3.29ns/op (137%)  | 4.71ns/op (100%)  |
| otter.Cache  | 7.41ns/op (358%)  | 10.7ns/op (505%)  | 15.2ns/op (633%)  | 64.0ns/op (1359%) |
| patrickmn    | 139ns/op (6715%)  | 151ns/op (7123%)  | 133ns/op (5542%)  | 158ns/op (3355%)  |
| bigcache     | 26.1ns/op (1261%) | 24.1ns/op (1137%) | 24.6ns/op (1025%) | n/a               |
| freecache    | 42.1ns/op (2034%) | 42.2ns/op (1991%) | 42.9ns/op (1788%) | failed            |
| fastcache    | 19.8ns/op (957%)  | 18.2ns/op (858%)  | 19.2ns/op (800%)  | 26.7ns/op (567%)  |

### Baseline Multiple Threads Memory Usage

1000000 items with 14 goroutines and 10% writes.
Byte caches (`bigcache`, `freecache`, `fastcache`) are used with binary encoding/decoding of a structure, which puts
them at a minor disadvantage for extra serialization work.

| Backend      | time/op          | MB/inuse      |
|--------------|------------------|---------------|
| sync.Map     | 9.64ns/op (100%) | 267MB (611%)  |
| shardedMap   | 42.2ns/op (438%) | 284MB (650%)  |
| mutexMap     | 221ns/op (2293%) | 227MB (519%)  |
| rwMutexMap   | 74.2ns/op (770%) | 227MB (519%)  |
| shardedMapOf | 41.4ns/op (429%) | 268MB (613%)  |
| ristretto    | 148ns/op (1535%) | 220MB (503%)  |
| xsync.Map    | 11.8ns/op (122%) | 142MB (325%)  |
| dlht.Map     | 12.3ns/op (128%) | 254MB (581%)  |
| otter.Cache  | 68.7ns/op (713%) | 182MB (416%)  |
| patrickmn    | 197ns/op (2044%) | 178MB (407%)  |
| bigcache     | n/a              | n/a           |
| freecache    | 53.3ns/op (553%) | 333MB (762%)  |
| fastcache    | 31.4ns/op (326%) | 43.7MB (100%) |

### Baseline Single Thread Read Only

Reading items from a cache with 10000 items using single goroutine.

| Backend      | time/op          |
|--------------|------------------|
| sync.Map     | 21.5ns/op (178%) |
| shardedMap   | 63.8ns/op (527%) |
| mutexMap     | 12.2ns/op (101%) |
| rwMutexMap   | 12.1ns/op (100%) |
| shardedMapOf | 64.6ns/op (534%) |
| ristretto    | 92.1ns/op (761%) |
| xsync.Map    | 26.5ns/op (219%) |
| dlht.Map     | 38.1ns/op (315%) |
| otter.Cache  | 90.1ns/op (745%) |
| patrickmn    | 65.9ns/op (545%) |
| bigcache     | 145ns/op (1198%) |
| freecache    | 135ns/op (1116%) |
| fastcache    | 118ns/op (975%)  |

## Server Machine

Benchmark is performed on `linux/amd64` server hardware (Intel(R) Xeon(R) CPU E5-2620 v4 @ 2.10GHz) from
`bench-report.txt`, with 32 benchmark goroutines in the multi-threaded runs.

### Baseline performance vs contextualized `ReadWriter` vs contextualized `Failover`

1000000 items with 32 goroutines and 10% writes.

| Backend      | Baseline         | ReadWriter       | Failover        |
|--------------|------------------|------------------|-----------------|
| ShardedMap   | 60.0ns/op (290%) | 60.9ns/op (209%) | 293ns/op (100%) |
| SyncMap      | 20.7ns/op (100%) | 29.1ns/op (100%) | 303ns/op (103%) |
| ShardedMapOf | 58.9ns/op (285%) | 58.9ns/op (202%) | 295ns/op (101%) |

### Typed Key Variants

1000000 items with 32 goroutines and 10% writes.

| Backend       | Baseline         | ReadWriter       | Failover        |
|---------------|------------------|------------------|-----------------|
| rawShardedMap | 46.5ns/op (182%) | 61.7ns/op (235%) | 291ns/op (107%) |
| SyncMapBy     | 25.5ns/op (100%) | 26.3ns/op (100%) | 274ns/op (100%) |
| ShardedMapBy  | 57.4ns/op (225%) | 57.2ns/op (217%) | 273ns/op (100%) |

### Baseline Multiple Threads With Partial Writes

Reading items from a cache with 10000 items using 32 goroutines and invoking additional writes for a fraction of reads.

| Backend       | 0% writes         | 0.1% writes       | 1% writes        | 10% writes       |
|---------------|-------------------|-------------------|------------------|------------------|
| sync.Map      | 4.43ns/op (100%)  | 4.50ns/op (100%)  | 5.47ns/op (100%) | 9.09ns/op (100%) |
| rawShardedMap | 15.9ns/op (359%)  | 22.9ns/op (509%)  | 29.2ns/op (534%) | 44.2ns/op (486%) |
| shardedMap    | 19.3ns/op (436%)  | 30.6ns/op (680%)  | 37.6ns/op (687%) | 53.8ns/op (592%) |
| mutexMap      | 309ns/op (6975%)  | 283ns/op (6289%)  | 326ns/op (5958%) | 469ns/op (5160%) |
| rwMutexMap    | 81.3ns/op (1835%) | 201ns/op (4467%)  | 217ns/op (3967%) | 195ns/op (2145%) |
| syncMapBy     | 9.10ns/op (205%)  | 10.3ns/op (229%)  | 10.7ns/op (196%) | 14.5ns/op (160%) |
| shardedMapBy  | 18.9ns/op (427%)  | 31.3ns/op (696%)  | 36.8ns/op (673%) | 53.6ns/op (590%) |
| shardedMapOf  | 19.4ns/op (438%)  | 34.2ns/op (760%)  | 37.7ns/op (689%) | 54.5ns/op (600%) |
| ristretto     | 22.2ns/op (501%)  | 30.7ns/op (682%)  | 49.8ns/op (910%) | 233ns/op (2563%) |
| xsync.Map     | 6.64ns/op (150%)  | 9.36ns/op (208%)  | 14.0ns/op (256%) | 18.5ns/op (204%) |
| dlht.Map      | 8.13ns/op (184%)  | 9.25ns/op (206%)  | 7.94ns/op (145%) | 10.4ns/op (114%) |
| otter.Cache   | 18.3ns/op (413%)  | 20.8ns/op (462%)  | 24.3ns/op (444%) | 266ns/op (2926%) |
| patrickmn     | 109ns/op (2460%)  | 198ns/op (4400%)  | 314ns/op (5740%) | 402ns/op (4422%) |
| bigcache      | 36.7ns/op (828%)  | 37.9ns/op (842%)  | 38.6ns/op (706%) | 56.5ns/op (622%) |
| freecache     | 49.2ns/op (1111%) | 51.2ns/op (1138%) | 51.0ns/op (932%) | 55.5ns/op (611%) |
| fastcache     | 30.7ns/op (693%)  | 34.6ns/op (769%)  | 36.3ns/op (664%) | 56.3ns/op (619%) |

### Baseline Multiple Threads Memory Usage

1000000 items with 32 goroutines and 10% writes.
Byte caches (`bigcache`, `freecache`, `fastcache`) are used with binary encoding/decoding of a structure, which puts
them at a minor disadvantage for extra serialization work.

| Backend       | time/op          | MB/inuse      |
|---------------|------------------|---------------|
| sync.Map      | 20.7ns/op (100%) | 267MB (611%)  |
| rawShardedMap | 46.5ns/op (225%) | 227MB (519%)  |
| shardedMap    | 60.0ns/op (290%) | 284MB (650%)  |
| mutexMap      | 560ns/op (2705%) | 227MB (519%)  |
| rwMutexMap    | 257ns/op (1242%) | 227MB (519%)  |
| syncMapBy     | 25.5ns/op (123%) | 298MB (682%)  |
| shardedMapBy  | 57.4ns/op (277%) | 231MB (529%)  |
| shardedMapOf  | 58.9ns/op (285%) | 268MB (613%)  |
| ristretto     | 274ns/op (1324%) | 225MB (515%)  |
| xsync.Map     | 21.2ns/op (102%) | 139MB (318%)  |
| dlht.Map      | 58.2ns/op (281%) | 250MB (572%)  |
| otter.Cache   | 241ns/op (1164%) | 179MB (410%)  |
| patrickmn     | 541ns/op (2614%) | 174MB (398%)  |
| bigcache      | 59.5ns/op (287%) | 359MB (822%)  |
| freecache     | 63.2ns/op (305%) | 333MB (762%)  |
| fastcache     | 62.8ns/op (303%) | 43.7MB (100%) |

### Baseline Single Thread Read Only

Reading items from a cache with 10000 items using single goroutine.

| Backend       | time/op          |
|---------------|------------------|
| sync.Map      | 99.1ns/op (137%) |
| rawShardedMap | 90.6ns/op (125%) |
| shardedMap    | 185ns/op (256%)  |
| mutexMap      | 72.4ns/op (100%) |
| rwMutexMap    | 72.2ns/op (100%) |
| syncMapBy     | 183ns/op (253%)  |
| shardedMapBy  | 185ns/op (256%)  |
| shardedMapOf  | 184ns/op (255%)  |
| ristretto     | 271ns/op (375%)  |
| xsync.Map     | 117ns/op (162%)  |
| dlht.Map      | 138ns/op (191%)  |
| otter.Cache   | 298ns/op (413%)  |
| patrickmn     | 206ns/op (285%)  |
| bigcache      | 560ns/op (776%)  |
| freecache     | 504ns/op (698%)  |
| fastcache     | 451ns/op (625%)  |

## Cross-Machine Observations

- `ReadWriter` shifts the most. On Apple M3 Max, `SyncMap` is the slowest of the three main backends at `112ns/op (269%)`, while on the Intel server it becomes the fastest at `29.1ns/op (100%)`. `ShardedMap` and `ShardedMapOf` stay close to each other on both machines.
- `Failover` is stable across machines. On both runs, the three main backends end up within `100-103%`, which suggests the failover layer dominates backend differences in this scenario.
- Baseline multi-thread results keep the same broad leaders, but the M3 Max run makes `sync.Map` look much more dominant. The Intel server still favors `sync.Map`, but the gap to `xsync.Map`, `dlht.Map`, and typed variants is smaller.
- Memory rankings are consistent. `fastcache` remains the clear `MB/inuse` winner on both machines, `xsync.Map` stays relatively compact, and `sync.Map` / sharded variants remain in the same general memory range.
- These are not pure hardware-only deltas. The Apple run used `14` goroutines in the multi-threaded sections, while the server run used `32`, so the comparison mixes CPU, OS, scheduler, and benchmark-concurrency effects.
