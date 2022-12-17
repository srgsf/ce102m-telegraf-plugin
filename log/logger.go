package log

import (
	"errors"
	"io"
	"log"
	"strings"
)

type LogLevel uint8

const (
	LvlError LogLevel = iota
	LvlWarning
	LvlInfo
	LvlDebug
)

var ErrInvalidLogLevel = errors.New("invalid log level")

func (l *LogLevel) UnmarshalText(b []byte) error {
	if len(b) < 4 {
		return ErrInvalidLogLevel
	}
	switch strings.ToLower(string(b)) {
	case "error":
		*l = LvlError
	case "warning":
		*l = LvlWarning
	case "info":
		*l = LvlInfo
	case "debug":
		*l = LvlDebug
	default:
		return ErrInvalidLogLevel
	}
	return nil
}

const logFlags = log.LstdFlags | log.Lmsgprefix

var (
	ERROR = log.New(io.Discard, "", logFlags)
	WARN  = log.New(io.Discard, "", logFlags)
	INFO  = log.New(io.Discard, "", logFlags)
	DEBUG = log.New(io.Discard, "", logFlags)
)

func Debugf(fmt string, args ...interface{}) {
	DEBUG.Printf(fmt, args...)
}

func Debug(v ...interface{}) {
	DEBUG.Println(v...)
}
func Warnf(fmt string, args ...interface{}) {
	WARN.Printf(fmt, args...)
}

func Warn(v ...interface{}) {
	WARN.Println(v...)
}
func Infof(fmt string, args ...interface{}) {
	INFO.Printf(fmt, args...)
}

func Info(v ...interface{}) {
	INFO.Println(v...)
}
func Errorf(fmt string, args ...interface{}) {
	ERROR.Printf(fmt, args...)
}

func Error(v ...interface{}) {
	ERROR.Println(v...)
}

func InitLoggers(w io.Writer, level LogLevel) {
	WARN = log.New(io.Discard, "", logFlags)
	INFO = log.New(io.Discard, "", logFlags)
	DEBUG = log.New(io.Discard, "", logFlags)

	ERROR = log.New(w, "E! ", logFlags)

	if level > LvlError {
		WARN = log.New(w, "W! ", logFlags)
	}
	if level > LvlWarning {
		INFO = log.New(w, "I! ", logFlags)
	}
	if level > LvlInfo {
		DEBUG = log.New(w, "D! ", logFlags)
	}
}
