package common

import "testing"

func TestRelayBufferPoolReturnsExpectedSize(t *testing.T) {
	bufPtr := RelayBufferPool.Get().(*[]byte)
	defer RelayBufferPool.Put(bufPtr)

	if len(*bufPtr) != BufferSize {
		t.Fatalf("unexpected buffer size: %d", len(*bufPtr))
	}
}
