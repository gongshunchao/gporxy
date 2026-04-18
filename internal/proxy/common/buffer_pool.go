package common

import "sync"

const BufferSize = 32 * 1024

var RelayBufferPool = sync.Pool{
	New: func() any {
		buf := make([]byte, BufferSize)
		return &buf
	},
}
