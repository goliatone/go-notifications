package templates

import (
	"context"
	"fmt"
	"strings"
	"sync"

	i18n "github.com/goliatone/go-i18n"
	"github.com/goliatone/go-notifications/pkg/domain"
	gotemplate "github.com/goliatone/go-template"
)

// Service coordinates template registration + rendering with locale-aware fallbacks.
type Service struct {
	renderer      *gotemplate.Engine
	registry      *registry
	helpers       *helperRegistry
	translator    i18n.Translator
	fallbacks     i18n.FallbackResolver
	defaultLocale string
	localeKey     string
	renderMu      sync.Mutex
}

// RenderRequest wraps the inputs needed to resolve and render a template variant.
type RenderRequest struct {
	Code    string
	Channel string
	Locale  string
	Data    map[string]any
}

// RenderResult returns the rendered subject/body along with metadata needed
// by downstream services (revision, source, fallback indicator).
type RenderResult struct {
	Subject      string
	Body         string
	Locale       string
	Revision     int
	Metadata     domain.JSONMap
	Source       domain.TemplateSource
	UsedFallback bool
}

type serviceOptions struct {
	defaultLocale  string
	fallbacks      i18n.FallbackResolver
	helperFuncs    []map[string]any
	rendererOpts   []gotemplate.Option
	missingHandler i18n.MissingTranslationHandler
	localeKey      string
}

// Option configures the template service.
type Option func(*serviceOptions)

// WithDefaultLocale overrides the locale used when lookups do not provide one.
func WithDefaultLocale(locale string) Option {
	return func(so *serviceOptions) {
		so.defaultLocale = locale
	}
}

// WithFallbackResolver wires a locale fallback resolver (e.g., es-MX -> es -> en).
func WithFallbackResolver(resolver i18n.FallbackResolver) Option {
	return func(so *serviceOptions) {
		so.fallbacks = resolver
	}
}

// WithHelperFuncs registers additional helper functions with the renderer.
func WithHelperFuncs(funcs map[string]any) Option {
	return func(so *serviceOptions) {
		if len(funcs) == 0 {
			return
		}
		so.helperFuncs = append(so.helperFuncs, funcs)
	}
}

// WithRendererOptions forwards options directly to go-template's renderer.
func WithRendererOptions(opts ...gotemplate.Option) Option {
	return func(so *serviceOptions) {
		so.rendererOpts = append(so.rendererOpts, opts...)
	}
}

// WithLocaleKey customizes the key injected into the data map to expose the locale.
func WithLocaleKey(key string) Option {
	return func(so *serviceOptions) {
		if key == "" {
			return
		}
		so.localeKey = key
	}
}

// WithMissingTranslationHandler customizes how go-i18n helpers surface missing keys.
func WithMissingTranslationHandler(handler i18n.MissingTranslationHandler) Option {
	return func(so *serviceOptions) {
		so.missingHandler = handler
	}
}

// NewService builds the template service wiring the helper registry, renderer,
// and localization translator together.
func NewService(translator i18n.Translator, opts ...Option) (*Service, error) {
	if translator == nil {
		return nil, ErrTranslatorRequired
	}

	settings := serviceOptions{
		localeKey: "locale",
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&settings)
		}
	}

	defaultLocale := strings.TrimSpace(settings.defaultLocale)
	if defaultLocale == "" {
		if provider, ok := translator.(interface{ DefaultLocale() string }); ok {
			defaultLocale = provider.DefaultLocale()
		}
	}
	if defaultLocale == "" {
		defaultLocale = "en"
	}

	rendererOpts := []gotemplate.Option{
		gotemplate.WithBaseDir("."),
	}
	rendererOpts = append(rendererOpts, settings.rendererOpts...)

	renderer, err := gotemplate.NewRenderer(rendererOpts...)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRendererConfig, err)
	}

	service := &Service{
		renderer:      renderer,
		registry:      newRegistry(),
		helpers:       newHelperRegistry(renderer),
		translator:    translator,
		fallbacks:     settings.fallbacks,
		defaultLocale: defaultLocale,
		localeKey:     settings.localeKey,
	}

	helperCfg := i18n.HelperConfig{
		LocaleKey:         service.localeKey,
		TemplateHelperKey: "t",
		OnMissing:         settings.missingHandler,
	}
	service.helpers.Register(i18n.TemplateHelpers(translator, helperCfg))

	for _, funcs := range settings.helperFuncs {
		service.helpers.Register(funcs)
	}

	return service, nil
}

// RegisterTemplates loads template variants into the service registry.
func (s *Service) RegisterTemplates(_ context.Context, templates ...domain.NotificationTemplate) {
	if s == nil {
		return
	}
	for _, tpl := range templates {
		s.registry.Upsert(tpl)
	}
}

// RegisterHelpers adds helper functions to the underlying renderer.
func (s *Service) RegisterHelpers(funcs map[string]any) {
	if s == nil {
		return
	}
	s.helpers.Register(funcs)
}

// Render fetches the appropriate template variant and produces localized content.
func (s *Service) Render(ctx context.Context, req RenderRequest) (RenderResult, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return RenderResult{}, err
		}
	}
	if s == nil {
		return RenderResult{}, ErrRendererConfig
	}
	if strings.TrimSpace(req.Code) == "" || strings.TrimSpace(req.Channel) == "" {
		return RenderResult{}, ErrInvalidRenderRequest
	}

	resolutionChain := s.localeChain(req.Locale)
	variant, resolvedLocale, err := s.registry.Resolve(req.Code, req.Channel, resolutionChain)
	if err != nil {
		return RenderResult{}, err
	}

	if variant.Subject() == "" || variant.Body() == "" {
		return RenderResult{}, fmt.Errorf("templates: template %s/%s missing subject/body", req.Code, req.Channel)
	}

	payload := cloneData(req.Data)
	payload[s.localeKey] = resolvedLocale

	if err := validateSchemaData(variant.Schema(), payload); err != nil {
		return RenderResult{}, err
	}

	s.renderMu.Lock()
	subject, err := s.renderer.RenderString(variant.Subject(), payload)
	if err != nil {
		s.renderMu.Unlock()
		return RenderResult{}, fmt.Errorf("templates: render subject: %w", err)
	}
	body, err := s.renderer.RenderString(variant.Body(), payload)
	s.renderMu.Unlock()
	if err != nil {
		return RenderResult{}, fmt.Errorf("templates: render body: %w", err)
	}

	return RenderResult{
		Subject:      subject,
		Body:         body,
		Locale:       resolvedLocale,
		Revision:     variant.Revision(),
		Metadata:     variant.Metadata(),
		Source:       variant.Source(),
		UsedFallback: !strings.EqualFold(resolvedLocale, strings.TrimSpace(req.Locale)),
	}, nil
}

func (s *Service) localeChain(requested string) []string {
	chain := make([]string, 0, 4)
	appendUnique := func(locale string) {
		if locale == "" {
			return
		}
		for _, existing := range chain {
			if strings.EqualFold(existing, locale) {
				return
			}
		}
		chain = append(chain, locale)
	}

	appendUnique(requested)
	if s.fallbacks != nil {
		for _, fb := range s.fallbacks.Resolve(requested) {
			appendUnique(fb)
		}
	}
	appendUnique(s.defaultLocale)
	appendUnique("en")
	return chain
}
