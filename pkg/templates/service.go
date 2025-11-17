package templates

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	i18n "github.com/goliatone/go-i18n"
	internaltemplates "github.com/goliatone/go-notifications/internal/templates"
	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/cache"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
)

// RenderRequest maps to the internal templates service request payload.
type RenderRequest = internaltemplates.RenderRequest

// RenderResult wraps the rendered subject/body pair returned by the internal service.
type RenderResult = internaltemplates.RenderResult

// Service exposes CRUD helpers and rendering facilities for notification templates.
type Service struct {
	repo          store.NotificationTemplateRepository
	cache         cache.Cache
	logger        logger.Logger
	engine        *internaltemplates.Service
	cacheTTL      time.Duration
	defaultLocale string
	fallbacks     i18n.FallbackResolver
}

// Dependencies wires repositories + translator dependencies.
type Dependencies struct {
	Repository    store.NotificationTemplateRepository
	Cache         cache.Cache
	Logger        logger.Logger
	Translator    i18n.Translator
	Fallbacks     i18n.FallbackResolver
	DefaultLocale string
	CacheTTL      time.Duration
}

// TemplateInput captures user-editable template fields.
type TemplateInput struct {
	Code        string
	Channel     string
	Locale      string
	Subject     string
	Body        string
	Description string
	Format      string
	Schema      domain.TemplateSchema
	Source      domain.TemplateSource
	Metadata    domain.JSONMap
}

var (
	errRepositoryRequired = errors.New("templates: repository is required")
	errTranslatorRequired = errors.New("templates: translator is required")
)

// New instantiates the templates facade using the provided dependencies.
func New(deps Dependencies) (*Service, error) {
	if deps.Repository == nil {
		return nil, errRepositoryRequired
	}
	if deps.Translator == nil {
		return nil, errTranslatorRequired
	}
	if deps.Cache == nil {
		deps.Cache = &cache.Nop{}
	}
	if deps.Logger == nil {
		deps.Logger = &logger.Nop{}
	}
	if deps.CacheTTL <= 0 {
		deps.CacheTTL = time.Minute
	}

	defaultLocale := strings.TrimSpace(deps.DefaultLocale)
	if defaultLocale == "" {
		if provider, ok := deps.Translator.(interface{ DefaultLocale() string }); ok {
			defaultLocale = provider.DefaultLocale()
		}
	}
	if defaultLocale == "" {
		defaultLocale = "en"
	}

	engine, err := internaltemplates.NewService(
		deps.Translator,
		internaltemplates.WithDefaultLocale(defaultLocale),
		internaltemplates.WithFallbackResolver(deps.Fallbacks),
	)
	if err != nil {
		return nil, err
	}

	return &Service{
		repo:          deps.Repository,
		cache:         deps.Cache,
		logger:        deps.Logger,
		engine:        engine,
		cacheTTL:      deps.CacheTTL,
		defaultLocale: defaultLocale,
		fallbacks:     deps.Fallbacks,
	}, nil
}

// RegisterHelpers exposes helper registration to callers.
func (s *Service) RegisterHelpers(funcs map[string]any) {
	if s == nil {
		return
	}
	s.engine.RegisterHelpers(funcs)
}

// Create persists a template variant and registers it for rendering.
func (s *Service) Create(ctx context.Context, input TemplateInput) (*domain.NotificationTemplate, error) {
	if s == nil {
		return nil, errRepositoryRequired
	}
	record, err := newDomainTemplate(input)
	if err != nil {
		return nil, err
	}
	record.Revision = 1

	if err := s.repo.Create(ctx, &record); err != nil {
		return nil, err
	}
	s.engine.RegisterTemplates(ctx, record)
	s.writeCache(ctx, record)
	return &record, nil
}

// Update mutates an existing template, bumping its revision for auditing.
func (s *Service) Update(ctx context.Context, input TemplateInput) (*domain.NotificationTemplate, error) {
	if s == nil {
		return nil, errRepositoryRequired
	}
	current, err := s.repo.GetByCodeAndLocale(ctx, strings.TrimSpace(input.Code), strings.TrimSpace(input.Locale), strings.TrimSpace(input.Channel))
	if err != nil {
		return nil, err
	}
	updated, err := mergeTemplateInput(*current, input)
	if err != nil {
		return nil, err
	}
	updated.Revision = current.Revision + 1

	if err := s.repo.Update(ctx, &updated); err != nil {
		return nil, err
	}
	s.engine.RegisterTemplates(ctx, updated)
	s.writeCache(ctx, updated)
	return &updated, nil
}

// Get fetches the persisted template variant and ensures the renderer has a copy.
func (s *Service) Get(ctx context.Context, code, channel, locale string) (*domain.NotificationTemplate, error) {
	tpl, err := s.loadTemplate(ctx, code, channel, locale)
	if err != nil {
		return nil, err
	}
	if tpl == nil {
		return nil, store.ErrNotFound
	}
	return tpl, nil
}

// ListByCode enumerates variants for a given template code (channel + locale fanout).
func (s *Service) ListByCode(ctx context.Context, code string, opts store.ListOptions) (store.ListResult[domain.NotificationTemplate], error) {
	result, err := s.repo.ListByCode(ctx, strings.TrimSpace(code), opts)
	if err != nil {
		return store.ListResult[domain.NotificationTemplate]{}, err
	}
	items := make([]domain.NotificationTemplate, len(result.Items))
	for i, tpl := range result.Items {
		items[i] = cloneTemplate(tpl)
	}
	return store.ListResult[domain.NotificationTemplate]{Items: items, Total: result.Total}, nil
}

// Render executes the template pipeline after ensuring the requested variant is loaded.
func (s *Service) Render(ctx context.Context, req RenderRequest) (RenderResult, error) {
	if err := s.ensureVariant(ctx, req.Code, req.Channel, req.Locale); err != nil {
		return RenderResult{}, err
	}
	return s.engine.Render(ctx, req)
}

func (s *Service) ensureVariant(ctx context.Context, code, channel, locale string) error {
	for _, candidate := range s.localeCandidates(locale) {
		if _, err := s.loadTemplate(ctx, code, channel, candidate); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				continue
			}
			return err
		}
		return nil
	}
	return store.ErrNotFound
}

func (s *Service) loadTemplate(ctx context.Context, code, channel, locale string) (*domain.NotificationTemplate, error) {
	if s == nil {
		return nil, errRepositoryRequired
	}
	key := cacheKey(code, channel, locale)
	if tpl := s.readCache(ctx, key); tpl != nil {
		s.engine.RegisterTemplates(ctx, *tpl)
		return tpl, nil
	}
	record, err := s.repo.GetByCodeAndLocale(ctx, strings.TrimSpace(code), strings.TrimSpace(locale), strings.TrimSpace(channel))
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, store.ErrNotFound
	}
	s.engine.RegisterTemplates(ctx, *record)
	s.writeCache(ctx, *record)
	clone := cloneTemplate(*record)
	return &clone, nil
}

func (s *Service) localeCandidates(requested string) []string {
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

func (s *Service) readCache(ctx context.Context, key string) *domain.NotificationTemplate {
	if key == "" {
		return nil
	}
	value, ok, err := s.cache.Get(ctx, key)
	if err != nil {
		s.logger.Warn("templates cache get failed", logger.Field{Key: "error", Value: err})
		return nil
	}
	if !ok {
		return nil
	}
	switch v := value.(type) {
	case domain.NotificationTemplate:
		clone := cloneTemplate(v)
		return &clone
	case *domain.NotificationTemplate:
		if v == nil {
			return nil
		}
		clone := cloneTemplate(*v)
		return &clone
	default:
		s.logger.Warn("templates cache returned unexpected type", logger.Field{Key: "type", Value: fmt.Sprintf("%T", value)})
		return nil
	}
}

func (s *Service) writeCache(ctx context.Context, tpl domain.NotificationTemplate) {
	if s.cacheTTL <= 0 {
		return
	}
	key := cacheKey(tpl.Code, tpl.Channel, tpl.Locale)
	if key == "" {
		return
	}
	if err := s.cache.Set(ctx, key, cloneTemplate(tpl), s.cacheTTL); err != nil {
		s.logger.Warn("templates cache set failed", logger.Field{Key: "error", Value: err})
	}
}

func cacheKey(code, channel, locale string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	channel = strings.ToLower(strings.TrimSpace(channel))
	locale = strings.ToLower(strings.TrimSpace(locale))
	if code == "" || channel == "" || locale == "" {
		return ""
	}
	return fmt.Sprintf("templates:%s:%s:%s", code, channel, locale)
}

func newDomainTemplate(input TemplateInput) (domain.NotificationTemplate, error) {
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return domain.NotificationTemplate{}, err
	}

	record := domain.NotificationTemplate{
		Code:        input.Code,
		Channel:     input.Channel,
		Locale:      input.Locale,
		Description: input.Description,
		Format:      input.Format,
		Subject:     input.Subject,
		Body:        input.Body,
		Schema:      sanitizeSchema(input.Schema),
		Source:      input.Source,
		Metadata:    cloneJSONMap(input.Metadata),
	}
	return record, nil
}

func mergeTemplateInput(base domain.NotificationTemplate, input TemplateInput) (domain.NotificationTemplate, error) {
	input = normalizeInput(input)
	if input.Code == "" {
		input.Code = base.Code
	}
	if input.Channel == "" {
		input.Channel = base.Channel
	}
	if input.Locale == "" {
		input.Locale = base.Locale
	}
	if input.Description == "" {
		input.Description = base.Description
	}
	if input.Format == "" {
		input.Format = base.Format
	}
	if input.Subject == "" {
		input.Subject = base.Subject
	}
	if input.Body == "" {
		input.Body = base.Body
	}
	if input.Source.Type == "" {
		input.Source = base.Source
	}
	if input.Metadata == nil {
		input.Metadata = base.Metadata
	}
	if input.Schema.IsZero() {
		input.Schema = base.Schema
	}
	if err := validateInput(input); err != nil {
		return domain.NotificationTemplate{}, err
	}
	base.Code = input.Code
	base.Channel = input.Channel
	base.Locale = input.Locale
	base.Description = input.Description
	base.Format = input.Format
	base.Subject = input.Subject
	base.Body = input.Body
	base.Source = input.Source
	base.Metadata = cloneJSONMap(input.Metadata)
	base.Schema = sanitizeSchema(input.Schema)
	return base, nil
}

func normalizeInput(input TemplateInput) TemplateInput {
	input.Code = strings.TrimSpace(input.Code)
	input.Channel = strings.TrimSpace(input.Channel)
	input.Locale = strings.TrimSpace(input.Locale)
	input.Subject = strings.TrimSpace(input.Subject)
	input.Body = strings.TrimSpace(input.Body)
	input.Description = strings.TrimSpace(input.Description)
	input.Format = strings.TrimSpace(input.Format)
	if input.Description == "" {
		input.Description = input.Code
	}
	if input.Format == "" {
		input.Format = "text/plain"
	}
	if input.Metadata == nil {
		input.Metadata = make(domain.JSONMap)
	}
	return input
}

func validateInput(input TemplateInput) error {
	if input.Code == "" {
		return errors.New("templates: code is required")
	}
	if input.Channel == "" {
		return errors.New("templates: channel is required")
	}
	if input.Locale == "" {
		return errors.New("templates: locale is required")
	}
	if input.Subject == "" && input.Source.Type == "" {
		return errors.New("templates: subject is required when source is empty")
	}
	if input.Body == "" && input.Source.Type == "" {
		return errors.New("templates: body is required when source is empty")
	}
	return nil
}

func sanitizeSchema(schema domain.TemplateSchema) domain.TemplateSchema {
	if schema.IsZero() {
		return schema
	}
	return domain.TemplateSchema{
		Required: dedupeStrings(schema.Required),
		Optional: dedupeStrings(schema.Optional),
	}
}

func dedupeStrings(list []string) []string {
	if len(list) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(list))
	result := make([]string, 0, len(list))
	for _, value := range list {
		key := strings.ToLower(strings.TrimSpace(value))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func cloneTemplate(tpl domain.NotificationTemplate) domain.NotificationTemplate {
	return domain.NotificationTemplate{
		RecordMeta:  tpl.RecordMeta,
		Code:        tpl.Code,
		Channel:     tpl.Channel,
		Locale:      tpl.Locale,
		Description: tpl.Description,
		Body:        tpl.Body,
		Subject:     tpl.Subject,
		Format:      tpl.Format,
		Revision:    tpl.Revision,
		Source:      tpl.Source,
		Schema:      tpl.Schema,
		Metadata:    cloneJSONMap(tpl.Metadata),
	}
}

func cloneJSONMap(src domain.JSONMap) domain.JSONMap {
	if len(src) == 0 {
		return nil
	}
	dst := make(domain.JSONMap, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
