package arena_test

import (
	"context"
	"runtime"
	"runtime/debug"
	"strconv"
	"testing"

	"github.com/bool64/cache"
	"github.com/bool64/cache/arena"
)

type Entity struct {
	I    int
	S    string
	Prev *Entity
}

func Test_Arena(t *testing.T) {
	runtime.GC()

	st := debug.GCStats{}
	debug.ReadGCStats(&st)

	println(st.NumGC, st.PauseTotal.String())

	var prev *Entity

	ctx := context.Background()
	c := arena.NewShardedMapOf[Entity]()
	for i := 0; i < 1e7; i++ {
		e := Entity{
			I:    i,
			S:    strconv.Itoa(i),
			Prev: prev,
		}

		prev = &e

		_ = c.Write(ctx, []byte(e.S), e)
	}

	for i := 0; i < 1e7; i++ {
		_, _ = c.Read(ctx, []byte(strconv.Itoa(i)))
	}

	debug.ReadGCStats(&st)
	println("DONE", st.NumGC, st.PauseTotal.String())
}

func Test_ShardedMap(t *testing.T) {
	runtime.GC()

	st := debug.GCStats{}
	debug.ReadGCStats(&st)

	println(st.NumGC, st.PauseTotal.String())

	var prev *Entity

	ctx := context.Background()
	c := cache.NewShardedMapOf[Entity]()
	for i := 0; i < 1e7; i++ {
		e := Entity{
			I:    i,
			S:    strconv.Itoa(i),
			Prev: prev,
		}

		prev = &e

		_ = c.Write(ctx, []byte(e.S), e)
	}

	for i := 0; i < 1e7; i++ {
		_, _ = c.Read(ctx, []byte(strconv.Itoa(i)))
	}

	debug.ReadGCStats(&st)
	println("DONE", st.NumGC, st.PauseTotal.String())
}
