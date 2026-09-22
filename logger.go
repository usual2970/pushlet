package pushlet

import (
	"fmt"
	"log"
)

// Logger is the minimal logging surface used by Pushlet connection handlers.
type Logger interface {
	Println(v ...any)
	WithField(key string, value any) Logger
}

// NewLogger constructs a [Logger] for one connection or request scope.
type NewLogger func() Logger

// NewDefaultLogger returns a logger that writes to the standard library log package.
func NewDefaultLogger() Logger {
	return &defaultLogger{
		fields: make(map[string]any),
	}
}

type defaultLogger struct {
	fields map[string]any
}

func (l *defaultLogger) Println(v ...any) {
	log.Println(append(l.fieldsToLog(), v...)...)
}

func (l *defaultLogger) WithField(key string, value any) Logger {
	l.fields[key] = value
	return l
}

func (l *defaultLogger) fieldsToLog() []any {
	var fields []any
	for k, v := range l.fields {
		fields = append(fields, fmt.Sprintf("%s: %v", k, v))
	}
	return fields
}
