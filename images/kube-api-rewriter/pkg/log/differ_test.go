/*
Copyright 2025 Flant JSC

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

package log

import (
	"context"
	"log/slog"
	"strings"
	"testing"
)

type captureHandler struct {
	level slog.Level
	attrs []slog.Attr
	logs  []slog.Record
}

var _ slog.Handler = (*captureHandler)(nil)

func newCaptureHandler(level slog.Level) *captureHandler {
	return &captureHandler{level: level}
}

func (h *captureHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *captureHandler) Handle(_ context.Context, record slog.Record) error {
	rec := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	if len(h.attrs) > 0 {
		rec.AddAttrs(h.attrs...)
	}
	record.Attrs(func(attr slog.Attr) bool {
		rec.AddAttrs(attr)
		return true
	})
	h.logs = append(h.logs, rec)
	return nil
}

func (h *captureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.attrs = append(append([]slog.Attr(nil), h.attrs...), attrs...)
	return &clone
}

func (h *captureHandler) WithGroup(string) slog.Handler {
	return h
}

func TestDebugBodyChangesNoDebug(t *testing.T) {
	handler := newCaptureHandler(slog.LevelInfo)
	logger := slog.New(handler)

	DebugBodyChanges(logger, "msg", "devices", []byte("in"), []byte("out"))

	if len(handler.logs) != 0 {
		t.Fatalf("expected no logs when debug disabled")
	}
}

func TestDebugBodyChangesBranches(t *testing.T) {
	tests := []struct {
		name       string
		in         []byte
		out        []byte
		validateFn func(t *testing.T, records []slog.Record)
	}{
		{
			name: "no rewrite",
			in:   []byte("in"),
			out:  nil,
			validateFn: func(t *testing.T, records []slog.Record) {
				if len(records) != 1 || !strings.Contains(records[0].Message, "no changes") {
					t.Fatalf("unexpected records: %+v", records)
				}
			},
		},
		{
			name: "empty bodies",
			in:   []byte{},
			out:  []byte{},
			validateFn: func(t *testing.T, records []slog.Record) {
				if len(records) != 1 || !strings.Contains(records[0].Message, "empty body") {
					t.Fatalf("unexpected message: %+v", records)
				}
			},
		},
		{
			name: "target produced data for empty input",
			in:   []byte{},
			out:  []byte(`{"kind":"GPUDevice"}`),
			validateFn: func(t *testing.T, records []slog.Record) {
				if len(records) != 2 {
					t.Fatalf("expected 2 records, got %d", len(records))
				}
				if !strings.Contains(records[0].Message, "possible bug") {
					t.Fatalf("unexpected message: %s", records[0].Message)
				}
				foundDump := false
				records[1].Attrs(func(attr slog.Attr) bool {
					if attr.Key == BodyDumpKey {
						foundDump = strings.Contains(attr.Value.String(), "GPUDevice")
					}
					return true
				})
				if !foundDump {
					t.Fatalf("expected body dump attr")
				}
			},
		},
		{
			name: "target erased body",
			in:   []byte(`{"apiVersion":"v1"}`),
			out:  []byte{},
			validateFn: func(t *testing.T, records []slog.Record) {
				if len(records) != 2 {
					t.Fatalf("expected 2 records, got %d", len(records))
				}
				if records[0].Level != slog.LevelError {
					t.Fatalf("expected error log")
				}
			},
		},
		{
			name: "diff success",
			in:   []byte(`{"apiVersion":"v1","kind":"GPUDevice","metadata":{"name":"a"}}`),
			out:  []byte(`{"apiVersion":"v1","kind":"GPUDevice","metadata":{"name":"b"}}`),
			validateFn: func(t *testing.T, records []slog.Record) {
				if len(records) != 1 {
					t.Fatalf("expected single log, got %d", len(records))
				}
				foundDiff := false
				records[0].Attrs(func(attr slog.Attr) bool {
					if attr.Key == BodyDiffKey {
						foundDiff = true
					}
					return true
				})
				if !foundDiff {
					t.Fatalf("diff attr not found")
				}
			},
		},
		{
			name: "diff error",
			in:   []byte(`{"broken":`),
			out:  []byte(`{}`),
			validateFn: func(t *testing.T, records []slog.Record) {
				if len(records) != 2 {
					t.Fatalf("expected 2 records, got %d", len(records))
				}
				if records[0].Level != slog.LevelError {
					t.Fatalf("expected error log")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler := newCaptureHandler(slog.LevelDebug)
			logger := slog.New(handler)
			DebugBodyChanges(logger, "msg", "virtualmachines", tc.in, tc.out)
			tc.validateFn(t, handler.logs)
		})
	}
}

func TestDebugBodyHead(t *testing.T) {
	handler := newCaptureHandler(slog.LevelDebug)
	logger := slog.New(handler)
	payload := []byte(strings.Repeat("a", 10))
	DebugBodyHead(logger, "demo", "virtualmachines", payload)
	if len(handler.logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(handler.logs))
	}
	var dump string
	handler.logs[0].Attrs(func(attr slog.Attr) bool {
		if attr.Key == BodyDumpKey {
			dump = attr.Value.String()
		}
		return true
	})
	if !strings.HasPrefix(dump, "[10]") {
		t.Fatalf("unexpected dump: %s", dump)
	}

	if got := headBytes(nil, 5); got != "<empty>" {
		t.Fatalf("expected <empty>, got %q", got)
	}
	if got := headBytes([]byte("abcd"), 10); got != "[4] abcd" {
		t.Fatalf("unexpected dump: %s", got)
	}

	handlerPatch := newCaptureHandler(slog.LevelDebug)
	loggerPatch := slog.New(handlerPatch)
	DebugBodyHead(loggerPatch, "demo", "patch", []byte("123456"))
	if len(handlerPatch.logs) != 1 {
		t.Fatalf("expected log for patch resource")
	}
}

func TestDiffEdgeCases(t *testing.T) {
	cases := []struct {
		name    string
		a, b    []byte
		want    string
		wantErr bool
	}{
		{"both nil", nil, nil, "<Empty>", false},
		{"only first", []byte(`{}`), nil, "<No rewrite was done>", false},
		{"only second", nil, []byte(`{}`), "", true},
		{"equal", []byte(`{"a":1}`), []byte(`{"a":1}`), "<Equal>", false},
		{"invalid first", []byte(`{`), []byte(`{}`), "", true},
		{"invalid second", []byte(`{}`), []byte(`{`), "", true},
		{"diff ok", []byte(`{"a":1}`), []byte(`{"a":2}`), "- 1", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Diff(tc.a, tc.b)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(got, tc.want) {
				t.Fatalf("unexpected diff: %q", got)
			}
		})
	}
}
