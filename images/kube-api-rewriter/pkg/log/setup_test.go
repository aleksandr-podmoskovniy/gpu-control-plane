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
	"io"
	stdlog "log/slog"
	"os"
	"strings"
	"testing"

	"log/slog"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = orig

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	return string(out)
}

func TestSetupDefaultLoggerFromEnvText(t *testing.T) {
	msg := captureStdout(t, func() {
		SetupDefaultLoggerFromEnv(Options{
			Level:  "info",
			Format: "text",
			Output: "stdout",
		})
		stdlog.Info("probe", stdlog.String("k", "v"))
	})

	if !strings.Contains(msg, "probe") {
		t.Fatalf("expected log output, got %q", msg)
	}
}

func TestSetupDefaultLoggerFromEnvJSON(t *testing.T) {
	msg := captureStdout(t, func() {
		SetupDefaultLoggerFromEnv(Options{
			Level:  "debug",
			Format: "json",
			Output: "stdout",
		})
		stdlog.Debug("json-probe")
	})
	if !strings.Contains(msg, "\"msg\":\"json-probe\"") {
		t.Fatalf("expected JSON log, got %q", msg)
	}
}

func TestDetectLogHelpers(t *testing.T) {
	if lvl := detectLogLevel("unknown"); lvl != DefaultLogLevel {
		t.Fatalf("unexpected level: %v", lvl)
	}
	if detectLogLevel("error") != slog.LevelError {
		t.Fatalf("expected error level")
	}
	if detectLogLevel("warn") != slog.LevelWarn {
		t.Fatalf("expected warn level")
	}
	if detectLogLevel("debug") != slog.LevelDebug {
		t.Fatalf("expected debug level")
	}

	if format := detectLogFormat("", slog.LevelDebug); format != DefaultDebugLogFormat {
		t.Fatalf("unexpected format: %s", format)
	}
	if format := detectLogFormat("", slog.LevelInfo); format != DefaultLogFormat {
		t.Fatalf("unexpected format for info level: %s", format)
	}
	if detectLogFormat("text", slog.LevelInfo) != TextLog {
		t.Fatalf("expected text format")
	}
	if detectLogFormat("json", slog.LevelInfo) != JSONLog {
		t.Fatalf("expected json format")
	}
	if detectLogFormat("pretty", slog.LevelInfo) != PrettyLog {
		t.Fatalf("expected pretty format")
	}

	oldOutput := DefaultLogOutput
	defer func() { DefaultLogOutput = oldOutput }()
	DefaultLogOutput = os.Stderr
	if detectLogOutput("unknown") != os.Stderr {
		t.Fatalf("expected default output to be returned")
	}
	if detectLogOutput("stdout") != os.Stdout {
		t.Fatalf("expected stdout writer")
	}
	if detectLogOutput("stderr") != os.Stderr {
		t.Fatalf("expected stderr writer")
	}
	if detectLogOutput("discard") != io.Discard {
		t.Fatalf("expected discard writer")
	}
}

func TestSetupHandlerFormats(t *testing.T) {
	if SetupHandler(Options{Format: "text"}) == nil {
		t.Fatalf("expected text handler")
	}
	if SetupHandler(Options{Format: "json"}) == nil {
		t.Fatalf("expected json handler")
	}
	if SetupHandler(Options{Format: "pretty"}) == nil {
		t.Fatalf("expected pretty handler")
	}
	if handler := SetupHandler(Options{Level: "debug"}); handler == nil {
		t.Fatalf("expected default handler for debug level")
	}
	if handler := SetupHandler(Options{}); handler == nil {
		t.Fatalf("expected handler for default options")
	}
	if handler := SetupHandler(Options{Format: "unknown"}); handler == nil {
		t.Fatalf("expected handler for fallback format")
	}
}
