//go:build go1.21
// +build go1.21

/*
Copyright 2021 The logr Authors.

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

package funcr

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/go-logr/logr"
)

func TestSlogSink(t *testing.T) {
	testCases := []struct {
		name      string
		withAttrs []any
		withGroup string
		args      []any
		expect    string
	}{{
		name:   "just msg",
		args:   makeKV(),
		expect: `{"logger":"","level":0,"msg":"msg"}`,
	}, {
		name:   "primitives",
		args:   makeKV("int", 1, "str", "ABC", "bool", true),
		expect: `{"logger":"","level":0,"msg":"msg","int":1,"str":"ABC","bool":true}`,
	}, {
		name:      "with attrs",
		withAttrs: makeKV("attrInt", 1, "attrStr", "ABC", "attrBool", true),
		args:      makeKV("int", 2),
		expect:    `{"logger":"","level":0,"msg":"msg","attrInt":1,"attrStr":"ABC","attrBool":true,"int":2}`,
	}, {
		name:      "with group",
		withGroup: "groupname",
		args:      makeKV("int", 1, "str", "ABC", "bool", true),
		expect:    `{"logger":"","level":0,"msg":"msg","groupname":{"int":1,"str":"ABC","bool":true}}`,
	}, {
		name:      "with attrs and group",
		withAttrs: makeKV("attrInt", 1, "attrStr", "ABC"),
		withGroup: "groupname",
		args:      makeKV("int", 3, "bool", true),
		expect:    `{"logger":"","level":0,"msg":"msg","attrInt":1,"attrStr":"ABC","groupname":{"int":3,"bool":true}}`,
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			capt := &capture{}
			logger := logr.New(newSink(capt.Func, NewFormatterJSON(Options{})))
			slogger := slog.New(logr.ToSlogHandler(logger))
			if len(tc.withAttrs) > 0 {
				slogger = slogger.With(tc.withAttrs...)
			}
			if tc.withGroup != "" {
				slogger = slogger.WithGroup(tc.withGroup)
			}
			slogger.Info("msg", tc.args...)
			if capt.log != tc.expect {
				t.Errorf("\nexpected %q\n     got %q", tc.expect, capt.log)
			}
		})
	}
}

func TestSlogSinkNestedGroups(t *testing.T) {
	capt := &capture{}
	logger := logr.New(newSink(capt.Func, NewFormatterJSON(Options{})))
	slogger := slog.New(logr.ToSlogHandler(logger))
	slogger = slogger.With("out", 0)
	slogger = slogger.WithGroup("g1").With("mid1", 1)
	slogger = slogger.WithGroup("g2").With("mid2", 2)
	slogger = slogger.WithGroup("g3").With("in", 3)
	slogger.Info("msg", "k", "v")

	expect := `{"logger":"","level":0,"msg":"msg","out":0,"g1":{"mid1":1,"g2":{"mid2":2,"g3":{"in":3,"k":"v"}}}}`
	if capt.log != expect {
		t.Errorf("\nexpected %q\n     got %q", expect, capt.log)
	}
}

func TestSlogSinkWithCaller(t *testing.T) {
	capt := &capture{}
	logger := logr.New(newSink(capt.Func, NewFormatterJSON(Options{LogCaller: All})))
	slogger := slog.New(logr.ToSlogHandler(logger))
	slogger.Error("msg", "int", 1)
	_, file, line, _ := runtime.Caller(0)
	expect := fmt.Sprintf(`{"logger":"","caller":{"file":%q,"line":%d},"msg":"msg","error":null,"int":1}`, filepath.Base(file), line-1)
	if capt.log != expect {
		t.Errorf("\nexpected %q\n     got %q", expect, capt.log)
	}
}
