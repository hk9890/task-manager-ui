package logging

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	stateDirName         = "bwb"
	logFilePrefix        = "bwb-"
	logFileSuffix        = ".log"
	rotationMaxSizeMB    = 10
	rotationMaxBackups   = 5
	rotationMaxAgeDays   = 30
	rotationCompress     = true
	defaultFileMode      = 0o644
	defaultDirectoryMode = 0o755
)

// Options configures logger construction.
type Options struct {
	Debug        bool
	Stderr       io.Writer
	StateDir     string
	SessionID    string
	ProjectRoot  string
	BuildVersion string
}

// Manager owns the application root logger and session metadata.
type Manager struct {
	root          *slog.Logger
	sessionID     string
	logPath       string
	closer        io.Closer
	stderrHandler *stderrHandler // retained so SetStderrSuppressed can toggle it
}

// New constructs the root logger with persistent JSON logs and stderr mirroring.
func New(opts Options) *Manager {
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	sessionID := strings.TrimSpace(opts.SessionID)
	if sessionID == "" {
		sessionID = generateSessionID()
	}

	sh := newStderrHandler(stderr, opts.Debug)
	handlers := []slog.Handler{sh}

	logPath, fileSink, err := buildPersistentSink(opts, sessionID)
	var closer io.Closer
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "bwb logging warning: persistent log unavailable; continuing with stderr-only logging: %v\n", err)
	} else {
		safe := newFailsafeSink(fileSink, stderr)
		handlers = append(handlers, newJSONFileHandler(safe, opts.Debug))
		closer = fileSink
	}

	root := slog.New(newTeeHandler(handlers...)).With(
		"session_id", sessionID,
		"project_root", strings.TrimSpace(opts.ProjectRoot),
		"build_version", strings.TrimSpace(opts.BuildVersion),
	)
	if opts.Debug {
		_, _ = fmt.Fprintf(stderr, "[bwb-debug] session_id=%s\n", sessionID)
	}

	return &Manager{
		root:          root,
		sessionID:     sessionID,
		logPath:       logPath,
		closer:        closer,
		stderrHandler: sh,
	}
}

// Logger returns the root logger.
func (m *Manager) Logger() *slog.Logger {
	if m == nil {
		return slog.Default()
	}
	return m.root
}

// Component returns a component-scoped logger.
func (m *Manager) Component(name string) *slog.Logger {
	logger := m.Logger()
	if strings.TrimSpace(name) == "" {
		return logger
	}
	return logger.With("component", name)
}

// LogPath returns the persistent log path. Empty means stderr-only fallback.
func (m *Manager) LogPath() string {
	if m == nil {
		return ""
	}
	return m.logPath
}

// SetStderrSuppressed controls whether the stderr handler emits log records.
//
// Call with true immediately before tea.Program.Run() to prevent slog writes
// from corrupting the alt-screen TTY during interactive mode. Call with false
// (or defer the call) after program.Run() returns so post-exit messages reach
// the terminal again.
//
// The persistent file sink is never affected; all records continue to be
// written to the log file regardless of this setting.
func (m *Manager) SetStderrSuppressed(suppressed bool) {
	if m == nil || m.stderrHandler == nil {
		return
	}
	m.stderrHandler.suppressed.Store(suppressed)
}

// Close closes any persistent sink resources.
func (m *Manager) Close() error {
	if m == nil || m.closer == nil {
		return nil
	}
	return m.closer.Close()
}

func buildPersistentSink(opts Options, sessionID string) (string, *lumberjack.Logger, error) {
	path, err := resolveLogPath(opts.StateDir, sessionID)
	if err != nil {
		return "", nil, err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, defaultFileMode)
	if err != nil {
		return "", nil, fmt.Errorf("open log file %q: %w", path, err)
	}
	_ = file.Close()

	sink := &lumberjack.Logger{
		Filename:   path,
		MaxSize:    rotationMaxSizeMB,
		MaxBackups: rotationMaxBackups,
		MaxAge:     rotationMaxAgeDays,
		Compress:   rotationCompress,
	}

	return path, sink, nil
}

// failsafeSink wraps an io.Writer (typically a lumberjack.Logger). On the
// first write error it emits one warning line to the fallback writer and then
// routes all subsequent writes there, ensuring silent failures are surfaced
// exactly once per session.
type failsafeSink struct {
	mu       sync.Mutex
	primary  io.Writer
	fallback io.Writer
	failed   bool
	warned   bool
}

func newFailsafeSink(primary io.Writer, fallback io.Writer) *failsafeSink {
	return &failsafeSink{primary: primary, fallback: fallback}
}

func (s *failsafeSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.failed {
		return s.fallback.Write(p)
	}

	n, err := s.primary.Write(p)
	if err != nil {
		s.failed = true
		if !s.warned {
			s.warned = true
			_, _ = fmt.Fprintf(s.fallback, "bwb logging warning: persistent sink write failed; switching to stderr-only logging: %v\n", err)
		}
		return s.fallback.Write(p)
	}
	return n, nil
}

// resolveLogPath returns the per-process log file path inside the bwb state
// directory. The filename is bwb-<sessionID>.log so that concurrent BWB
// processes never share a log file or its lumberjack rotation state, which
// would cause torn JSON Lines records across a rotation boundary.
func resolveLogPath(stateDirOverride string, sessionID string) (string, error) {
	stateDir := strings.TrimSpace(stateDirOverride)
	if stateDir == "" {
		var err error
		stateDir, err = defaultUserStateDir()
		if err != nil {
			return "", fmt.Errorf("resolve user state dir: %w", err)
		}
	}

	bwbStateDir := filepath.Join(stateDir, stateDirName)
	if err := os.MkdirAll(bwbStateDir, defaultDirectoryMode); err != nil {
		return "", fmt.Errorf("create state directory %q: %w", bwbStateDir, err)
	}

	id := strings.TrimSpace(sessionID)
	if id == "" {
		// Fallback: use the process ID when no session ID is available.
		id = fmt.Sprintf("pid%d", os.Getpid())
	}
	fileName := logFilePrefix + id + logFileSuffix
	return filepath.Join(bwbStateDir, fileName), nil
}

func defaultUserStateDir() (string, error) {
	if xdg := strings.TrimSpace(os.Getenv("XDG_STATE_HOME")); xdg != "" {
		return xdg, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state"), nil
}

func newJSONFileHandler(writer io.Writer, debug bool) slog.Handler {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}

	return slog.NewJSONHandler(writer, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			switch attr.Key {
			case slog.TimeKey:
				attr.Key = "timestamp"
			case slog.MessageKey:
				attr.Key = "message"
			}
			return attr
		},
	})
}

// randReader is the source of entropy for session ID generation. Replaced in
// tests to simulate rand.Read failures.
var randReader = func(b []byte) (int, error) { return rand.Read(b) }

// generateSessionID returns an 8-character hex session identifier. On rand.Read
// failure it appends the current nanosecond timestamp in hex to ensure uniqueness
// across concurrent processes or degraded-entropy restarts.
func generateSessionID() string {
	raw := make([]byte, 4)
	if _, err := randReader(raw); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(raw)
}

type teeHandler struct {
	handlers []slog.Handler
}

func newTeeHandler(handlers ...slog.Handler) slog.Handler {
	copyHandlers := append([]slog.Handler(nil), handlers...)
	return &teeHandler{handlers: copyHandlers}
}

func (h *teeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *teeHandler) Handle(ctx context.Context, record slog.Record) error {
	for _, handler := range h.handlers {
		if !handler.Enabled(ctx, record.Level) {
			continue
		}
		if err := handler.Handle(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

func (h *teeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	result := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		result = append(result, handler.WithAttrs(attrs))
	}
	return &teeHandler{handlers: result}
}

func (h *teeHandler) WithGroup(name string) slog.Handler {
	result := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		result = append(result, handler.WithGroup(name))
	}
	return &teeHandler{handlers: result}
}

type stderrHandler struct {
	mu         *sync.Mutex  // shared across all derived handlers; allocated once in newStderrHandler
	suppressed *atomic.Bool // shared across all derived handlers; when true, Enabled and Handle are no-ops
	writer     io.Writer
	debugOn    bool
	attrs      []slog.Attr
	group      string
}

func newStderrHandler(writer io.Writer, debug bool) *stderrHandler {
	return &stderrHandler{mu: &sync.Mutex{}, suppressed: &atomic.Bool{}, writer: writer, debugOn: debug}
}

func (h *stderrHandler) Enabled(_ context.Context, level slog.Level) bool {
	if h.suppressed.Load() {
		return false
	}
	if level >= slog.LevelWarn {
		return true
	}
	if !h.debugOn {
		return false
	}
	return level == slog.LevelDebug || level == slog.LevelInfo
}

func (h *stderrHandler) Handle(_ context.Context, record slog.Record) error {
	if h.suppressed.Load() {
		return nil
	}
	parts := make([]string, 0, 8)
	if record.Level < slog.LevelWarn {
		parts = append(parts, "[bwb-debug]", record.Message)
	} else {
		parts = append(parts, strings.ToLower(record.Level.String())+":", record.Message)
	}

	for _, attr := range h.attrs {
		parts = append(parts, formatAttr(h.group, attr))
	}
	record.Attrs(func(attr slog.Attr) bool {
		parts = append(parts, formatAttr(h.group, attr))
		return true
	})

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := fmt.Fprintln(h.writer, strings.Join(parts, " "))
	return err
}

func (h *stderrHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	copyAttrs := append(append([]slog.Attr(nil), h.attrs...), attrs...)
	return &stderrHandler{
		mu:         h.mu,         // share the same mutex; do not allocate a fresh one
		suppressed: h.suppressed, // share the same suppression flag
		writer:     h.writer,
		debugOn:    h.debugOn,
		attrs:      copyAttrs,
		group:      h.group,
	}
}

func (h *stderrHandler) WithGroup(name string) slog.Handler {
	nextGroup := name
	if strings.TrimSpace(h.group) != "" {
		nextGroup = h.group + "." + name
	}
	return &stderrHandler{
		mu:         h.mu,         // share the same mutex; do not allocate a fresh one
		suppressed: h.suppressed, // share the same suppression flag
		writer:     h.writer,
		debugOn:    h.debugOn,
		attrs:      append([]slog.Attr(nil), h.attrs...),
		group:      nextGroup,
	}
}

func formatAttr(group string, attr slog.Attr) string {
	key := attr.Key
	if strings.TrimSpace(group) != "" {
		key = group + "." + key
	}
	return fmt.Sprintf("%s=%v", key, attr.Value.Any())
}
