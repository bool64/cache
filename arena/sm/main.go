package main

import (
	"context"
	"encoding/json"
	"runtime"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/bool64/cache"
)

type Entity struct {
	I    int
	S    string
	Prev *Entity
}

func main() {
	st := debug.GCStats{}
	m := runtime.MemStats{}
	s := time.Now()
	debug.ReadGCStats(&st)
	runtime.ReadMemStats(&m)

	var prev *Entity

	ctx := context.Background()
	c := cache.NewShardedMapOf[*Entity]()
	for i := 0; i < 1e7; i++ {
		e := Entity{
			I:    i,
			S:    strconv.Itoa(i),
			Prev: prev,
		}

		prev = &e

		_ = c.Write(ctx, []byte(e.S), &e)
	}

	debug.ReadGCStats(&st)
	runtime.ReadMemStats(&m)

	println("CACHE READY", st.NumGC, st.PauseTotal.String(), m.HeapInuse/(1024*1024), time.Since(s).String(), m.NumGC, m.PauseTotalNs, m.TotalAlloc)

	sem := make(chan struct{}, 50)

	for {
		time.Sleep(time.Millisecond)

		sem <- struct{}{}

		go func() {
			defer func() {
				<-sem
			}()

			o := make([]byte, 1e4)
			var v map[string]interface{}
			_ = json.Unmarshal([]byte(`{"glossary":{"title":"example glossary","GlossDiv":{"title":"S","GlossList":{"GlossEntry":{"ID":"SGML","SortAs":"SGML","GlossTerm":"Standard Generalized Markup Language","Acronym":"SGML","Abbrev":"ISO 8879:1986","GlossDef":{"para":"A meta-markup language, used to create markup languages such as DocBook.","GlossSeeAlso":["GML","XML"]},"GlossSee":"markup"}}}}}`), &v)
			_, _ = c.Read(ctx, []byte(strconv.Itoa(len(v)+len(o))))
		}()

		if time.Since(s) > 30*time.Second {
			break
		}
	}

	debug.ReadGCStats(&st)
	runtime.ReadMemStats(&m)

	println("DONE", st.NumGC, st.PauseTotal.String(), m.HeapInuse/(1024*1024), time.Since(s).String(), m.NumGC, m.PauseTotalNs, m.TotalAlloc)
}

// CACHE READY 11 761.773µs 1704 6.090179653s
// DONE 19 5.742181ms 3742 30.000467679s
