package extensions

import (
	"bytes"
	"sync"
)

type boundedBuffer struct {
	mu        sync.Mutex
	contents  bytes.Buffer
	maximum   int
	truncated bool
}

func newBoundedBuffer(maximum int) *boundedBuffer {
	return &boundedBuffer{maximum: maximum}
}

func (b *boundedBuffer) Write(value []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	written := len(value)
	remaining := b.maximum - b.contents.Len()
	if remaining > 0 {
		if len(value) > remaining {
			value = value[:remaining]
		}
		_, _ = b.contents.Write(value)
	}
	if written > remaining {
		b.truncated = true
	}
	return written, nil
}

func (b *boundedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	value := b.contents.String()
	if b.truncated {
		const suffix = "\n[stderr truncated]"
		if b.maximum <= len(suffix) {
			return suffix[:max(0, b.maximum)]
		}
		if len(value) > b.maximum-len(suffix) {
			value = value[:b.maximum-len(suffix)]
		}
		value += suffix
	}
	return value
}
