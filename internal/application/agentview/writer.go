package agentview

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// bufferSize bounds how many events may wait for the disk. A full buffer drops the
// newest event: a lost view costs a number, a blocked request costs a caller.
const bufferSize = 4096

// lateWriteCutoff is how long after its UTC day ends an event may still be appended to
// that day's file. Past it the event is dropped: the rollup reads a day once SettleAfter
// has passed, and a line appended after that read would change the file's signature and
// have the whole day counted a second time. It must stay below SettleAfter.
const lateWriteCutoff = 10 * time.Minute

// Writer appends events to one file per UTC day under dir. One goroutine owns the files;
// handlers only send on a channel. Each event is a single write(2) of one line to an
// O_APPEND file, which Linux appends atomically for regular files, so the two blue/green
// server instances can share a day's file without interleaving lines.
type Writer struct {
	dir     string
	now     func() time.Time
	ch      chan Event
	dropped atomic.Int64
	done    chan struct{}
	// mu orders Write against Close: a Write after Close would send on a closed channel,
	// and a server whose shutdown timed out still has handlers in flight.
	mu     sync.RWMutex
	closed bool
}

// NewWriter starts a writer under dir, or returns nil (a no-op Sink) when dir is empty.
func NewWriter(dir string) *Writer {
	return newWriter(dir, time.Now)
}

func newWriter(dir string, now func() time.Time) *Writer {
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		log.Printf("agentview: create %s: %v", dir, err)
	}
	w := &Writer{dir: dir, now: now, ch: make(chan Event, bufferSize), done: make(chan struct{})}
	go w.run()
	return w
}

// Write queues e, or drops it when the buffer is full. Safe on a nil Writer.
func (w *Writer) Write(e Event) {
	if w == nil {
		return
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.closed {
		w.dropped.Add(1)
		return
	}
	select {
	case w.ch <- e:
	default:
		w.dropped.Add(1)
	}
}

// Dropped is how many events were dropped since start: by a full buffer, after Close, or
// too late for their day's file.
func (w *Writer) Dropped() int64 {
	if w == nil {
		return 0
	}
	return w.dropped.Load()
}

// Close flushes the queued events and stops the writer. Safe on a nil Writer, safe to
// call twice, and a Write after it is dropped (and counted) rather than a panic.
func (w *Writer) Close() {
	if w == nil {
		return
	}
	w.mu.Lock()
	if !w.closed {
		w.closed = true
		close(w.ch)
	}
	w.mu.Unlock()
	<-w.done
}

func (w *Writer) run() {
	defer close(w.done)
	files := dayWriter{dir: w.dir}
	defer files.close()
	for e := range w.ch {
		dayEnd := e.TS.UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)
		if w.now().Sub(dayEnd) > lateWriteCutoff {
			w.dropped.Add(1)
			continue
		}
		files.write(e)
	}
}

// dayWriter holds the one open day file the drain goroutine appends to.
type dayWriter struct {
	dir  string
	f    *os.File
	name string
	// failed is the file name an open last failed for, so a missing or unwritable
	// directory logs once per day's file rather than once per event.
	failed string
}

func (d *dayWriter) write(e Event) {
	line, err := json.Marshal(e)
	if err != nil {
		return
	}
	if n := FileName(e.TS); n != d.name {
		d.close()
		d.name = n
		d.f, err = os.OpenFile(filepath.Join(d.dir, n), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
		if err != nil {
			if d.failed != n {
				log.Printf("agentview: open %s: %v", n, err)
				d.failed = n
			}
			d.name = "" // retry the open on the next event
			return
		}
	}
	if _, err := d.f.Write(append(line, '\n')); err != nil {
		log.Printf("agentview: write %s: %v", d.name, err)
	}
}

// close closes the open day file. A failed close cannot lose lines already written, so
// it is logged rather than returned.
func (d *dayWriter) close() {
	if d.f == nil {
		return
	}
	if err := d.f.Close(); err != nil {
		log.Printf("agentview: close %s: %v", d.name, err)
	}
	d.f = nil
}
