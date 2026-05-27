package runtime

import (
	"io"
	"log/slog"

	"github.com/rs/zerolog"
)

func newZeroLoggerFromSlog(logger *slog.Logger) zerolog.Logger {
	if logger == nil {
		return zerolog.New(io.Discard)
	}
	return zerolog.New(io.Discard)
}
