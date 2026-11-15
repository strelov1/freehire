package catalogexport

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

// fakeBucket is an in-memory blobstore.Bucket.
type fakeBucket struct {
	mu   sync.Mutex
	objs map[string][]byte
}

func newFake() *fakeBucket { return &fakeBucket{objs: map[string][]byte{}} }

func (f *fakeBucket) Put(_ context.Context, key, _ string, r io.Reader, _ int64) error {
	b, err := io.ReadAll(r)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objs[key] = b
	return err
}

func (f *fakeBucket) PutStream(ctx context.Context, key, ct string, r io.Reader) error {
	return f.Put(ctx, key, ct, r, -1)
}

func (f *fakeBucket) Get(_ context.Context, key string) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.objs[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (f *fakeBucket) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objs, key)
	return nil
}

func (f *fakeBucket) List(_ context.Context, prefix string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var keys []string
	for k := range f.objs {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys, nil
}

func (f *fakeBucket) PresignGet(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://bucket.test/" + key, nil
}
