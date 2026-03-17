package salamoonder

import (
	"fmt"
	"os"
	"sync"
	"time"
)

const (
	LevelDEBUG   = 10
	LevelINFO    = 20
	LevelWARNING = 30
	LevelERROR   = 40
)

var (
	globalLevel = LevelINFO
	loggersMu   sync.Mutex
	loggers     = map[string]*Logger{}
)

type Logger struct {
	name string
}

func GetLogger(name string) *Logger {
	loggersMu.Lock()
	defer loggersMu.Unlock()
	if l, ok := loggers[name]; ok {
		return l
	}
	l := &Logger{name: name}
	loggers[name] = l
	return l
}

func SetLevel(level int) {
	globalLevel = level
}

func SetLevelByName(name string) {
	switch name {
	case "DEBUG":
		globalLevel = LevelDEBUG
	case "INFO":
		globalLevel = LevelINFO
	case "WARNING":
		globalLevel = LevelWARNING
	case "ERROR":
		globalLevel = LevelERROR
	default:
		globalLevel = LevelINFO
	}
}

func timestamp() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

func (l *Logger) log(levelName string, levelValue int, msg string) {
	if levelValue < globalLevel {
		return
	}
	line := fmt.Sprintf("%s - %s - %s - %s", timestamp(), l.name, levelName, msg)
	if levelValue >= LevelERROR {
		fmt.Fprintln(os.Stderr, line)
	} else {
		fmt.Println(line)
	}
}

func (l *Logger) Debug(format string, args ...interface{}) {
	l.log("DEBUG", LevelDEBUG, fmt.Sprintf(format, args...))
}

func (l *Logger) Info(format string, args ...interface{}) {
	l.log("INFO", LevelINFO, fmt.Sprintf(format, args...))
}

func (l *Logger) Warning(format string, args ...interface{}) {
	l.log("WARNING", LevelWARNING, fmt.Sprintf(format, args...))
}

func (l *Logger) Error(format string, args ...interface{}) {
	l.log("ERROR", LevelERROR, fmt.Sprintf(format, args...))
}
