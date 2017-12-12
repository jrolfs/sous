package logging

import (
	"bytes"
	"fmt"
	"runtime"
	"strings"
	"time"
)

type (
	// CallerInfo describes the source of a log message.
	// It should be included in almost every message, to capture information about
	// when and where the message was created.
	CallerInfo struct {
		callTime    time.Time
		goroutineID string
		callers     []uintptr
		excluding   []Excluder
	}

	// Excluder excludes a frame from being reported.
	Excluder interface {
		exclude(f runtime.Frame) bool
	}

	excludedFrame struct {
		function string
		file     string
		sensible bool
	}

	excludeFile string
)

var notGCI excludedFrame

// GetCallerInfo captures a CallerInfo based on where it's called.
func GetCallerInfo(excluding ...Excluder) CallerInfo {
	if !notGCI.sensible {
		notGCI = NotHere().(excludedFrame)
	}

	callers := make([]uintptr, 10)
	runtime.Callers(2, callers)

	prefixlen := len("goroutine ")
	buf := make([]byte, prefixlen+20)
	runtime.Stack(buf, false)
	buf = buf[prefixlen:]
	idx := bytes.IndexByte(buf, ' ')
	if idx != -1 {
		buf = buf[:idx]
	}
	return CallerInfo{
		callTime:    time.Now(),
		goroutineID: string(buf),
		callers:     callers,
		excluding:   append(excluding, notGCI),
	}
}

// ExcludeMe can be used to exclude other handler functions from the reportable stack
// example: msg.CallerInfo.ExcludeMe()
func (info *CallerInfo) ExcludeMe() {
	ex := excludeCaller()
	info.excluding = append(info.excluding, ex)
}

// Exclude path is used to exclude paths that contain a pattern
func (info *CallerInfo) ExcludePathPattern(pat string) {
	info.excluding = append(info.excluding, excludeFile(pat))
}

// EachField calls f repeatedly with name/value pairs that capture what CallerInfo knows about the message.
func (info CallerInfo) EachField(f FieldReportFn) {
	unknown := true
	frames := runtime.CallersFrames(info.callers)

	var aframe, frame runtime.Frame
	var stack string
	var more bool

	for aframe, more = frames.Next(); more; aframe, more = frames.Next() {
		stack = fmt.Sprintf("%s%s %s:%d\n", stack, aframe.Function, aframe.File, aframe.Line)

		if unknown && info.reportableFrame(aframe) {
			frame = aframe
			unknown = false
		}
	}

	f("@timestamp", info.callTime.UTC().Format(time.RFC3339))
	f("thread-name", info.goroutineID)
	f("call-stack-trace", stack)

	if frame.Function == "" {
		f("call-stack-function", "<unknown>")
	} else {
		f("call-stack-function", frame.Function)
	}

	if frame.File == "" {
		f("call-stack-file", "<unknown>")
	} else {
		f("call-stack-file", frame.File)
	}

	f("call-stack-line-number", frame.Line)
}

func (info CallerInfo) reportableFrame(f runtime.Frame) bool {
	if strings.Index(f.File, "<autogenerated>") != -1 {
		return false
	}

	if strings.Index(f.File, "go/src/runtime") != -1 {
		return false
	}

	for _, ex := range info.excluding {
		if ex.exclude(f) {
			return false
		}
	}
	return true
}

// NotHere returns the frame of its caller to exclude from search as the source of a log entry
func NotHere() Excluder {
	return excludeCaller()
}

func excludeCaller() excludedFrame {
	var frame runtime.Frame
	pc, _, _, ok := runtime.Caller(2) // 1 would be NotHere()
	if ok {
		fms := runtime.CallersFrames([]uintptr{pc})
		frame, _ = fms.Next()
	}

	return excludedFrame{
		function: frame.Function,
		file:     frame.File,
		sensible: ok,
	}
}

func (ef excludedFrame) exclude(f runtime.Frame) bool {
	// frame.Function should be sufficient - they're qualified with package (and type)
	if ef.sensible && ef.function == f.Function {
		return true
	}
	return false
}

func (pat excludeFile) exclude(f runtime.Frame) bool {
	return strings.Contains(f.File, string(pat))
}
