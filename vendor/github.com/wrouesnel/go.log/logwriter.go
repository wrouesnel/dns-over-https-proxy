
package log

import (
	"bytes"
	"io"
)

// Implements an io.Writer which writes its output to the given logging
// function. This allows, for example, easy streaming of command logs to
// the standard logging functions.
type LogWriter struct {
	logFunc func(args ...interface{})

	buffer []byte
	lineIn chan []byte
}

// Creates a new io.Writer which writes to the log output. Takes a log function
// to use for writing output.
func NewLogWriter(logFunc func(args ...interface{})) io.Writer {
	this := new(LogWriter)
	this.logFunc = logFunc
	this.lineIn = make(chan []byte)

	// Launch go-routine to handle log outputs
	go func() {
		for p := range this.lineIn {
			this.buffer = append(this.buffer, p...)

			lines := bytes.Split(this.buffer, []byte("\n"))

			// Log all lines except last unterminated line
			for _, line := range lines[:len(lines)-1] {
				this.logFunc(string(line))
			}

			// Set last unterminated line as the last line
			this.buffer = lines[len(lines)-1]
		}
	}()

	return io.Writer(this)
}

func (this *LogWriter) Write(p []byte) (n int, err error) {
	this.lineIn <- p // Send all writes to ingress channel
	return len(p), nil
}