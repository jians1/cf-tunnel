package runtime

import (
	"io"

	"github.com/rs/zerolog"
)

func newTestZeroLogger() zerolog.Logger {
	return zerolog.New(io.Discard)
}
