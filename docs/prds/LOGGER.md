# Logging Refactor Overview (go-logger Compatible)

This refactor aligns the internal logging contract with `go-logger` and adds a default logger implementation that uses `fmt.Printf`. It also updates all logging call sites to use key/value args, matching the go-logger style.

## What Changed

1. Logger interface now matches go-logger
   - Replaced the old `logger.Field`-based interface with:
     - `Trace/Debug/Info/Warn/Error/Fatal(msg string, args ...any)`
     - `WithContext(ctx context.Context) Logger`
   - Added optional interfaces:
     - `LoggerProvider` (`GetLogger(name string) Logger`)
     - `FieldsLogger` (`WithFields(map[string]any) Logger`)

2. Default logger implementation
   - Added `BasicLogger` that writes via `fmt.Printf`.
   - Added `logger.Default()` to provide a usable logger when none is supplied.

3. Default wiring
   - All services/adapters that used `&logger.Nop{}` as fallback now use `logger.Default()`.

4. Logging calls updated
   - All `logger.Field{Key, Value}` usage was replaced with key/value arguments:
     - Before: `logger.Field{Key: "error", Value: err}`
     - After: `"error", err`

5. Linting updated
   - Log-safety lint now inspects logger call args for secret keys, not `logger.Field` literals.

## Why This Matters

- Direct compatibility with `go-logger` (no wrapper required).
- Consistent logging API across packages.
- Safe default behavior with a usable stdout logger.

## How to Apply in Other Packages

### 1) Define the interface (go-logger compatible)

```go
type Logger interface {
	Trace(msg string, args ...any)
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	Fatal(msg string, args ...any)
	WithContext(ctx context.Context) Logger
}
```

### 2) Add a default logger

```go
func Default() Logger {
	return NewBasicLogger() // or equivalent
}
```

### 3) Update service constructors

```go
if deps.Logger == nil {
	deps.Logger = logger.Default()
}
```

### 4) Update logging calls

```go
// before
lgr.Warn("resolve failed", logger.Field{Key: "error", Value: err})

// after
lgr.Warn("resolve failed", "error", err)
```

### 5) Update tests/examples

- Replace any custom logger that implements the old interface.
- Ensure it implements `Trace/Debug/Info/Warn/Error/Fatal + WithContext`.

## Example Basic Logger (fmt.Printf)

```go
type BasicLogger struct{}

func (l *BasicLogger) Trace(msg string, args ...any) { l.log("TRACE", msg, args...) }
func (l *BasicLogger) Debug(msg string, args ...any) { l.log("DEBUG", msg, args...) }
func (l *BasicLogger) Info(msg string, args ...any)  { l.log("INFO", msg, args...) }
func (l *BasicLogger) Warn(msg string, args ...any)  { l.log("WARN", msg, args...) }
func (l *BasicLogger) Error(msg string, args ...any) { l.log("ERROR", msg, args...) }
func (l *BasicLogger) Fatal(msg string, args ...any) { l.log("FATAL", msg, args...) }
func (l *BasicLogger) WithContext(ctx context.Context) Logger { return l }

func (l *BasicLogger) log(level, msg string, args ...any) {
	fmt.Printf("[%s] %s %v\n", level, msg, args)
}
```

## Summary

- Interface matches `go-logger`.
- Default logger exists and is always safe to use.
- Structured data passed as key/value args.
- No direct dependency on `go-logger`, but fully compatible.
