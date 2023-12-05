//go:build go1.21
// +build go1.21

/*
Copyright 2023 The logr Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package logr_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"runtime"
	"strings"
	"testing"
	"testing/slogtest"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
)

var debugWithoutTime = &slog.HandlerOptions{
	ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
		if a.Key == "time" {
			return slog.Attr{}
		}
		return a
	},
	Level: slog.LevelDebug,
}

func ExampleFromSlogHandler() {
	logrLogger := logr.FromSlogHandler(slog.NewTextHandler(os.Stdout, debugWithoutTime))

	logrLogger.Info("hello world")
	logrLogger.Error(errors.New("fake error"), "ignore me")
	logrLogger.WithValues("x", 1, "y", 2).WithValues("str", "abc").WithName("foo").WithName("bar").V(4).Info("with values, verbosity and name")

	// Output:
	// level=INFO msg="hello world"
	// level=ERROR msg="ignore me" err="fake error"
	// level=DEBUG msg="with values, verbosity and name" x=1 y=2 str=abc logger=foo/bar
}

func ExampleToSlogHandler() {
	funcrLogger := funcr.New(func(prefix, args string) {
		if prefix != "" {
			fmt.Fprintln(os.Stdout, prefix, args)
		} else {
			fmt.Fprintln(os.Stdout, args)
		}
	}, funcr.Options{
		Verbosity: 10,
	})

	slogLogger := slog.New(logr.ToSlogHandler(funcrLogger))
	slogLogger.Info("hello world")
	slogLogger.Error("ignore me", "err", errors.New("fake error"))
	slogLogger.With("x", 1, "y", 2).WithGroup("group").With("str", "abc").Warn("with values and group")

	slogLogger = slog.New(logr.ToSlogHandler(funcrLogger.V(int(-slog.LevelDebug))))
	slogLogger.Info("info message reduced to debug level")

	// Output:
	// "level"=0 "msg"="hello world"
	// "msg"="ignore me" "error"=null "err"="fake error"
	// "level"=0 "msg"="with values and group" "x"=1 "y"=2 "group.str"="abc"
	// "level"=4 "msg"="info message reduced to debug level"
}

func TestWithCallDepth(t *testing.T) {
	debugWithCaller := *debugWithoutTime
	debugWithCaller.AddSource = true
	var buffer bytes.Buffer
	logger := logr.FromSlogHandler(slog.NewTextHandler(&buffer, &debugWithCaller))

	logHelper := func(logger logr.Logger) {
		logger.WithCallDepth(1).Info("hello")
	}

	logHelper(logger)
	_, file, line, _ := runtime.Caller(0)
	expectedSource := fmt.Sprintf("%s:%d", path.Base(file), line-1)
	actual := buffer.String()
	if !strings.Contains(actual, expectedSource) {
		t.Errorf("expected log entry with %s as caller source code location, got instead:\n%s", expectedSource, actual)
	}
}

func TestJSONHandler(t *testing.T) {
	testSlog(t, func(buffer *bytes.Buffer) logr.Logger {
		handler := slog.NewJSONHandler(buffer, nil)
		sink := testSlogSink{handler: handler}
		return logr.New(sink)
	})
}

var _ logr.LogSink = testSlogSink{}
var _ logr.SlogSink = testSlogSink{}

// testSlogSink is only used through slog and thus doesn't need to implement the
// normal LogSink methods.
type testSlogSink struct {
	handler slog.Handler
}

func (s testSlogSink) Init(logr.RuntimeInfo)                  {}
func (s testSlogSink) Enabled(int) bool                       { return true }
func (s testSlogSink) Error(error, string, ...interface{})    {}
func (s testSlogSink) Info(int, string, ...interface{})       {}
func (s testSlogSink) WithName(string) logr.LogSink           { return s }
func (s testSlogSink) WithValues(...interface{}) logr.LogSink { return s }

func (s testSlogSink) Handle(ctx context.Context, record slog.Record) error {
	return s.handler.Handle(ctx, record)
}
func (s testSlogSink) WithAttrs(attrs []slog.Attr) logr.SlogSink {
	return testSlogSink{handler: s.handler.WithAttrs(attrs)}
}
func (s testSlogSink) WithGroup(name string) logr.SlogSink {
	return testSlogSink{handler: s.handler.WithGroup(name)}
}

func TestFuncrHandler(t *testing.T) {
	fn := func(buffer *bytes.Buffer) logr.Logger {
		printfn := func(obj string) {
			fmt.Fprintln(buffer, obj)
		}
		opts := funcr.Options{
			LogTimestamp: true,
			Verbosity:    10,
			RenderBuiltinsHook: func(kvList []any) []any {
				mappedKVList := make([]any, len(kvList))
				for i := 0; i < len(kvList); i += 2 {
					key := kvList[i]
					switch key {
					case "ts":
						mappedKVList[i] = "time"
					default:
						mappedKVList[i] = key
					}
					mappedKVList[i+1] = kvList[i+1]
				}
				return mappedKVList
			},
		}
		return funcr.NewJSON(printfn, opts)
	}
	exceptions := []string{
		"a Handler should ignore a zero Record.Time",                     // Time is generated by sink.
		"a Handler should handle Group attributes",                       // funcr doesn't.
		"a Handler should inline the Attrs of a group with an empty key", // funcr doesn't know about groups.
		"a Handler should not output groups for an empty Record",         // Relies on WithGroup. Text may change, see https://go.dev/cl/516155
		"a Handler should handle the WithGroup method",                   // logHandler does by prefixing keys, which is not what the test expects.
		"a Handler should handle multiple WithGroup and WithAttr calls",  // Same.
		"a Handler should call Resolve on attribute values in groups",    // funcr doesn't do that and slogHandler can't do it for it.
	}
	testSlog(t, fn, exceptions...)
}

func testSlog(t *testing.T, createLogger func(buffer *bytes.Buffer) logr.Logger, exceptions ...string) {
	var buffer bytes.Buffer
	logger := createLogger(&buffer)
	handler := logr.ToSlogHandler(logger)
	err := slogtest.TestHandler(handler, func() []map[string]any {
		var ms []map[string]any
		for _, line := range bytes.Split(buffer.Bytes(), []byte{'\n'}) {
			if len(line) == 0 {
				continue
			}
			var m map[string]any
			if err := json.Unmarshal(line, &m); err != nil {
				t.Fatal(err)
			}
			ms = append(ms, m)
		}
		return ms
	})

	// Correlating failures with individual test cases is hard with the current API.
	// See https://github.com/golang/go/issues/61758
	t.Logf("Output:\n%s", buffer.String())
	if err != nil {
		if err, ok := err.(interface {
			Unwrap() []error
		}); ok {
			for _, err := range err.Unwrap() {
				if !containsOne(err.Error(), exceptions...) {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		} else {
			// Shouldn't be reached, errors from errors.Join can be split up.
			t.Errorf("Unexpected errors:\n%v", err)
		}
	}
}

func containsOne(hay string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(hay, needle) {
			return true
		}
	}
	return false
}

func TestDiscard(t *testing.T) {
	logger := slog.New(logr.ToSlogHandler(logr.Discard()))
	logger.WithGroup("foo").With("x", 1).Info("hello")
}

func TestConversion(t *testing.T) {
	d := logr.Discard()
	d2 := logr.FromSlogHandler(logr.ToSlogHandler(d))
	expectEqual(t, d, d2)

	e := logr.Logger{}
	e2 := logr.FromSlogHandler(logr.ToSlogHandler(e))
	expectEqual(t, e, e2)

	f := funcr.New(func(prefix, args string) {}, funcr.Options{})
	f2 := logr.FromSlogHandler(logr.ToSlogHandler(f))
	expectEqual(t, f, f2)

	text := slog.NewTextHandler(io.Discard, nil)
	text2 := logr.ToSlogHandler(logr.FromSlogHandler(text))
	expectEqual(t, text, text2)

	text3 := logr.ToSlogHandler(logr.FromSlogHandler(text).V(1))
	if handler, ok := text3.(interface {
		GetLevel() slog.Level
	}); ok {
		expectEqual(t, handler.GetLevel(), slog.Level(1))
	} else {
		t.Errorf("Expected a slogHandler which implements V(1), got instead: %T %+v", text3, text3)
	}
}

func expectEqual(t *testing.T, expected, actual any) {
	if expected != actual {
		t.Helper()
		t.Errorf("Expected %T %+v, got instead: %T %+v", expected, expected, actual, actual)
	}
}
