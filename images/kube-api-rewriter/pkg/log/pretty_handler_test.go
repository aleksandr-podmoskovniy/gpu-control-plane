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
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestDefaultCustomHandler(t *testing.T) {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		//Level:       nil,
		//ReplaceAttr: nil,
	})))

	logg := slog.With(
		slog.Group("properties",
			slog.Int("width", 4000),
			slog.Int("height", 3000),
			slog.String("format", "jpeg"),
			slog.Group("nestedprops",
				slog.String("arg", "val"),
			),
		),
		slog.String("azaz", "foo"),
	)
	logg.Info("message with group",
		slog.Group("properties",
			slog.Int("width", 6000),
		),
	)

	// set PrettyHandler as default
	//dbgHandler := NewPrettyHandler(os.Stdout, nil)
	dbgHandler := NewPrettyHandler(os.Stdout, &slog.HandlerOptions{AddSource: true})

	slog.SetDefault(slog.New(dbgHandler))

	logger := slog.With(
		slog.String("arg1", "val1"),
		slog.String("body.diff", "+-+-+-+\n++--++--\n  + qwe\n  - azaz"),
		slog.String("body.dump", "{ \"debug\": true }"),
		slog.Group("properties",
			slog.Int("width", 6000),
		),
	)

	logger.Info("info message")

	logger = slog.With(
		slog.String("arg1", "val1"),
		slog.String("body.diff", "+-+-+-+"),
	)
	logger.WithGroup("properties").Info("info message",
		slog.Int("width", 6000),
	)
}

func TestNewPrettyHandlerNilOptions(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := NewPrettyHandler(buf, nil)
	if handler.opts == nil {
		t.Fatalf("expected handler options to be initialized")
	}
	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
	if err := handler.Handle(context.Background(), rec); err != nil {
		t.Fatalf("handle error: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatalf("expected handler to write output")
	}
}

func TestSuppressDefaultAttrs(t *testing.T) {
	called := false
	next := func(groups []string, a slog.Attr) slog.Attr {
		called = true
		if len(groups) != 1 || groups[0] != "group" {
			t.Fatalf("unexpected groups: %v", groups)
		}
		return a
	}
	fn := suppressDefaultAttrs(next)
	if attr := fn(nil, slog.String(slog.TimeKey, "ignored")); attr.Key != "" {
		t.Fatalf("expected default attr to be suppressed")
	}
	res := fn([]string{"group"}, slog.String("custom", "value"))
	if res.Value.String() != "value" {
		t.Fatalf("unexpected attr: %v", res)
	}
	if !called {
		t.Fatalf("expected next to be invoked")
	}
}

type handlerFunc struct {
	handle    func(context.Context, slog.Record) error
	withAttrs func([]slog.Attr) slog.Handler
	withGroup func(string) slog.Handler
}

func (h handlerFunc) Enabled(context.Context, slog.Level) bool { return true }
func (h handlerFunc) Handle(ctx context.Context, r slog.Record) error {
	if h.handle != nil {
		return h.handle(ctx, r)
	}
	return nil
}
func (h handlerFunc) WithAttrs(attrs []slog.Attr) slog.Handler {
	if h.withAttrs != nil {
		return h.withAttrs(attrs)
	}
	return h
}
func (h handlerFunc) WithGroup(name string) slog.Handler {
	if h.withGroup != nil {
		return h.withGroup(name)
	}
	return h
}

func TestPrettyHandlerGatherAttrsErrors(t *testing.T) {
	handler := NewPrettyHandler(io.Discard, nil)

	handler.jh = handlerFunc{
		handle: func(context.Context, slog.Record) error {
			return fmt.Errorf("boom")
		},
	}
	if _, err := handler.gatherAttrs(context.Background(), slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)); err == nil {
		t.Fatalf("expected error from inner handler")
	}

	handler.jh = handlerFunc{
		handle: func(_ context.Context, _ slog.Record) error {
			_, _ = handler.jhb.WriteString("not-json")
			return nil
		},
	}
	if _, err := handler.gatherAttrs(context.Background(), slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)); err == nil {
		t.Fatalf("expected json unmarshal error")
	}
}

func TestPrettyHandlerHandleComplex(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := NewPrettyHandler(buf, &slog.HandlerOptions{AddSource: true})

	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "complex", 0)
	if pc, _, _, ok := runtime.Caller(0); ok {
		rec.PC = pc
	}
	rec.AddAttrs(
		slog.Any("list", []any{1, "two"}),
		slog.Any("grouped", map[string]any{"foo": "bar"}),
		slog.String(BodyDiffKey, "{-old,+new}"),
		slog.String(BodyDumpKey, "{\"key\":\"value\"}"),
	)

	if err := handler.Handle(context.Background(), rec); err != nil {
		t.Fatalf("handle error: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatalf("expected output written")
	}
}
