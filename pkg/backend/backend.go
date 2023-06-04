package backend

import "io"

type Backend interface {
	io.ReaderAt
	io.WriterAt

	Size() (int64, error)
	Sync() error

	// stringer interface, to provide info about this backend, mainly for logging
	String() string
}
