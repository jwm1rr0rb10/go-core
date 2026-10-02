package validator

import (
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	playground "github.com/go-playground/validator/v10"
	"github.com/jwm1rr0rb10/go-errors"
)

// DefaultDateLayout is the layout of the "date" tag unless WithDateLayout
// overrides it.
const DefaultDateLayout = time.DateOnly

// Option configures an Engine.
type Option func(*engineConfig)

type engineConfig struct {
	dateLayout string
	jsonNames  bool
	custom     map[string]playground.Func
}

// WithDateLayout sets the time layout checked by the "date" struct tag.
// Empty strings in tagged fields are always accepted; combine with
// "required" to forbid them.
func WithDateLayout(layout string) Option {
	return func(c *engineConfig) { c.dateLayout = layout }
}

// WithJSONFieldNames reports fields by their json tag name ("email") instead
// of the Go field name ("Email"). Fields tagged json:"-" keep the Go name.
func WithJSONFieldNames() Option {
	return func(c *engineConfig) { c.jsonNames = true }
}

// WithValidation registers a custom struct tag.
func WithValidation(tag string, fn playground.Func) Option {
	return func(c *engineConfig) {
		if c.custom == nil {
			c.custom = make(map[string]playground.Func)
		}
		c.custom[tag] = fn
	}
}

// Engine validates structs using tags. It is safe for concurrent use and
// caches struct metadata, so create one at startup and share it.
type Engine struct {
	v *playground.Validate
}

// NewEngine creates an Engine. It fails only if a custom tag cannot be
// registered (for example an empty tag name).
func NewEngine(opts ...Option) (*Engine, error) {
	cfg := engineConfig{dateLayout: DefaultDateLayout}
	for _, opt := range opts {
		opt(&cfg)
	}

	v := playground.New(playground.WithRequiredStructEnabled())
	if cfg.jsonNames {
		v.RegisterTagNameFunc(jsonTagName)
	}

	layout := cfg.dateLayout
	if err := v.RegisterValidation("date", func(fl playground.FieldLevel) bool {
		s := fl.Field().String()
		if s == "" {
			return true
		}
		_, err := time.Parse(layout, s)
		return err == nil
	}); err != nil {
		return nil, errors.Wrap(err, "validator: register date tag")
	}
	for tag, fn := range cfg.custom {
		if err := v.RegisterValidation(tag, fn); err != nil {
			return nil, errors.Wrapf(err, "validator: register tag %q", tag)
		}
	}
	return &Engine{v: v}, nil
}

func jsonTagName(f reflect.StructField) string {
	name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
	if name == "" || name == "-" {
		return f.Name
	}
	return name
}

// Struct validates s against its `validate` tags. It returns nil, a
// ValidationError listing every failed field, or the underlying error when s
// is not a struct (or pointer to one).
//
// Nested fields are named by their path without the root type, for example
// "Address.City".
func (e *Engine) Struct(s any) error {
	err := e.v.Struct(s)
	if err == nil {
		return nil
	}
	fieldErrs, ok := errors.AsType[playground.ValidationErrors](err)
	if !ok {
		return err
	}
	fields := make(ErrorFields, len(fieldErrs))
	for _, fe := range fieldErrs {
		name := fe.Namespace()
		if _, rest, found := strings.Cut(name, "."); found {
			name = rest
		}
		fields[name] = message(fe)
	}
	return ValidationError{Fields: fields}
}

// Var validates a single value against a tag expression such as
// "required,email", reporting failures under field.
func (e *Engine) Var(field string, value any, tag string) error {
	err := e.v.Var(value, tag)
	if err == nil {
		return nil
	}
	fieldErrs, ok := errors.AsType[playground.ValidationErrors](err)
	if !ok || len(fieldErrs) == 0 {
		return err
	}
	return fieldError(field, message(fieldErrs[0]))
}

// StructValidator returns a Validator that runs e.Struct(s).
func (e *Engine) StructValidator(s any) Validator {
	return ValidatorFunc(func() error { return e.Struct(s) })
}

func message(fe playground.FieldError) string {
	msg := "field validation for '" + fe.Field() + "' failed on the '" + fe.Tag() + "' tag"
	if p := fe.Param(); p != "" {
		msg += " (" + p + ")"
	}
	return msg
}

var (
	defaultEngine atomic.Pointer[Engine]
	defaultOnce   sync.Once
)

// Default returns the package-level Engine used by StructValidator. Unless
// replaced with SetDefault or New, it is created on first use with
// DefaultDateLayout.
func Default() *Engine {
	if e := defaultEngine.Load(); e != nil {
		return e
	}
	defaultOnce.Do(func() {
		e, err := NewEngine()
		if err != nil {
			panic(err) // unreachable: the built-in options always register
		}
		defaultEngine.CompareAndSwap(nil, e)
	})
	return defaultEngine.Load()
}

// SetDefault replaces the package-level Engine. It is safe to call
// concurrently with validation; a nil e is ignored.
func SetDefault(e *Engine) {
	if e != nil {
		defaultEngine.Store(e)
	}
}

// New configures the package-level Engine so that the "date" tag uses
// structDateFormat. Calling it is optional; later calls replace the engine.
//
// Prefer NewEngine and pass the Engine explicitly in new code.
func New(structDateFormat string) error {
	e, err := NewEngine(WithDateLayout(structDateFormat))
	if err != nil {
		return err
	}
	SetDefault(e)
	return nil
}

// StructValidator returns a Validator that checks s with the package-level
// Engine (see Default). It never panics, even if New was not called.
func StructValidator(s any) Validator {
	return ValidatorFunc(func() error { return Default().Struct(s) })
}
