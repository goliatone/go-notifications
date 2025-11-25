package exportready

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/goliatone/go-notifications/pkg/templates"
)

// Dependencies required to install export-ready assets.
type Dependencies struct {
	Definitions store.NotificationDefinitionRepository
	Templates   *templates.Service
}

// Options allow callers to customize codes and channel strings.
type Options struct {
	Namespace             string
	DefinitionName        string
	DefinitionDescription string
	Channels              []string

	EmailSubject   string
	EmailBody      string
	EmailCTALabel  string
	EmailIcon      string
	InAppSubject   string
	InAppBody      string
	InAppCTALabel  string
	InAppIcon      string
	DefinitionMeta domain.JSONMap
	TemplateMeta   domain.JSONMap
}

// Result exposes the registered assets for callers.
type Result struct {
	DefinitionCode string
	DefinitionID   string
	EmailCode      string
	EmailID        string
	InAppCode      string
	InAppID        string
}

// Register installs (or updates) the export-ready definition and templates.
func Register(ctx context.Context, deps Dependencies, opts Options) (Result, error) {
	if deps.Definitions == nil {
		return Result{}, errors.New("exportready: Definitions repository is required")
	}
	if deps.Templates == nil {
		return Result{}, errors.New("exportready: Templates service is required")
	}

	def := buildDefinition(opts)
	tpls := buildTemplates(opts)
	tpls = filterTemplatesByChannels(tpls, def.Channels)
	def.TemplateKeys = templateKeysFor(tpls)

	installedDef, err := upsertDefinition(ctx, deps.Definitions, def)
	if err != nil {
		return Result{}, err
	}

	installedEmail, err := upsertTemplate(ctx, deps.Templates, emailTemplateFor(tpls))
	if err != nil {
		return Result{}, err
	}
	installedInApp, err := upsertTemplate(ctx, deps.Templates, inAppTemplateFor(tpls))
	if err != nil {
		return Result{}, err
	}

	return Result{
		DefinitionCode: installedDef.Code,
		DefinitionID:   installedDef.ID.String(),
		EmailCode:      codeOrEmpty(installedEmail),
		EmailID:        idOrEmpty(installedEmail),
		InAppCode:      codeOrEmpty(installedInApp),
		InAppID:        idOrEmpty(installedInApp),
	}, nil
}

func buildDefinition(opts Options) domain.NotificationDefinition {
	base := Definition()
	if opts.Namespace != "" {
		base.Code = namespaced(opts.Namespace, base.Code)
		base.TemplateKeys = domain.StringList{
			"email:" + namespaced(opts.Namespace, EmailTemplateCode),
			"in-app:" + namespaced(opts.Namespace, InAppTemplateCode),
		}
	}
	if len(opts.Channels) > 0 {
		base.Channels = normalizeChannels(opts.Channels)
	}
	if opts.DefinitionName != "" {
		base.Name = opts.DefinitionName
	}
	if opts.DefinitionDescription != "" {
		base.Description = opts.DefinitionDescription
	}
	base.Metadata = mergeJSON(base.Metadata, opts.DefinitionMeta)
	return base
}

func buildTemplates(opts Options) []domain.NotificationTemplate {
	base := Templates()
	email := baseTemplateFor(base, "email")
	inapp := baseTemplateFor(base, "in-app")

	if opts.Namespace != "" {
		email.Code = namespaced(opts.Namespace, email.Code)
		inapp.Code = namespaced(opts.Namespace, inapp.Code)
	}

	if opts.EmailSubject != "" {
		email.Subject = opts.EmailSubject
	}
	if opts.EmailBody != "" {
		email.Body = opts.EmailBody
	}
	email.Metadata = mergeJSON(email.Metadata, opts.TemplateMeta)
	if label := defaultValue(opts.EmailCTALabel, "Download"); label != "" {
		email.Metadata["cta_label"] = label
	}
	if opts.EmailIcon != "" {
		email.Metadata["icon"] = opts.EmailIcon
	}

	if opts.InAppSubject != "" {
		inapp.Subject = opts.InAppSubject
	}
	if opts.InAppBody != "" {
		inapp.Body = opts.InAppBody
	}
	inapp.Metadata = mergeJSON(inapp.Metadata, opts.TemplateMeta)
	if label := defaultValue(opts.InAppCTALabel, "Open"); label != "" {
		inapp.Metadata["cta_label"] = label
	}
	if opts.InAppIcon != "" {
		inapp.Metadata["icon"] = opts.InAppIcon
	}

	return []domain.NotificationTemplate{email, inapp}
}

func upsertDefinition(ctx context.Context, repo store.NotificationDefinitionRepository, desired domain.NotificationDefinition) (*domain.NotificationDefinition, error) {
	existing, err := repo.GetByCode(ctx, desired.Code)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("exportready: get definition: %w", err)
	}
	if existing == nil {
		if err := repo.Create(ctx, &desired); err != nil {
			return nil, fmt.Errorf("exportready: create definition: %w", err)
		}
		return &desired, nil
	}

	updated := *existing
	updated.Name = desired.Name
	updated.Description = desired.Description
	updated.Severity = desired.Severity
	updated.Category = desired.Category
	updated.Channels = desired.Channels
	updated.TemplateKeys = desired.TemplateKeys
	updated.Metadata = mergeJSON(desired.Metadata, existing.Metadata)

	if definitionsEqual(*existing, updated) {
		return existing, nil
	}

	if err := repo.Update(ctx, &updated); err != nil {
		return nil, fmt.Errorf("exportready: update definition: %w", err)
	}
	return &updated, nil
}

func upsertTemplate(ctx context.Context, svc *templates.Service, desired domain.NotificationTemplate) (*domain.NotificationTemplate, error) {
	if strings.TrimSpace(desired.Code) == "" || strings.TrimSpace(desired.Channel) == "" {
		return nil, nil
	}
	current, err := svc.Get(ctx, desired.Code, desired.Channel, desired.Locale)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("exportready: get template %s/%s: %w", desired.Code, desired.Channel, err)
	}
	if current == nil {
		record, err := svc.Create(ctx, templates.TemplateInput{
			Code:        desired.Code,
			Channel:     desired.Channel,
			Locale:      desired.Locale,
			Subject:     desired.Subject,
			Body:        desired.Body,
			Description: desired.Description,
			Format:      desired.Format,
			Schema:      desired.Schema,
			Metadata:    desired.Metadata,
		})
		if err != nil {
			return nil, fmt.Errorf("exportready: create template %s/%s: %w", desired.Code, desired.Channel, err)
		}
		return record, nil
	}

	mergedMeta := mergeJSON(desired.Metadata, current.Metadata)
	if templatesEqual(*current, desired, mergedMeta) {
		return current, nil
	}

	updated, err := svc.Update(ctx, templates.TemplateInput{
		Code:        desired.Code,
		Channel:     desired.Channel,
		Locale:      desired.Locale,
		Subject:     desired.Subject,
		Body:        desired.Body,
		Description: desired.Description,
		Format:      desired.Format,
		Schema:      desired.Schema,
		Metadata:    mergedMeta,
	})
	if err != nil {
		return nil, fmt.Errorf("exportready: update template %s/%s: %w", desired.Code, desired.Channel, err)
	}
	return updated, nil
}

func mergeJSON(primary, secondary domain.JSONMap) domain.JSONMap {
	if len(primary) == 0 && len(secondary) == 0 {
		return nil
	}
	out := make(domain.JSONMap, len(primary)+len(secondary))
	for k, v := range secondary {
		out[k] = v
	}
	for k, v := range primary {
		out[k] = v
	}
	return out
}

func definitionsEqual(a, b domain.NotificationDefinition) bool {
	return strings.EqualFold(a.Code, b.Code) &&
		a.Name == b.Name &&
		a.Description == b.Description &&
		stringListsEqual(a.Channels, b.Channels) &&
		stringListsEqual(a.TemplateKeys, b.TemplateKeys) &&
		jsonEqual(a.Metadata, b.Metadata)
}

func templatesEqual(existing domain.NotificationTemplate, desired domain.NotificationTemplate, mergedMeta domain.JSONMap) bool {
	a := normalizeTemplate(existing)
	b := normalizeTemplate(domain.NotificationTemplate{
		Code:     desired.Code,
		Channel:  desired.Channel,
		Locale:   desired.Locale,
		Subject:  desired.Subject,
		Body:     desired.Body,
		Format:   desired.Format,
		Schema:   desired.Schema,
		Metadata: mergedMeta,
	})

	return a.Code == b.Code &&
		a.Channel == b.Channel &&
		a.Locale == b.Locale &&
		a.Subject == b.Subject &&
		a.Body == b.Body &&
		a.Format == b.Format &&
		jsonEqual(a.Metadata, b.Metadata) &&
		schemaEqual(a.Schema, b.Schema)
}

func stringListsEqual(a, b domain.StringList) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]int, len(a))
	for _, entry := range a {
		seen[strings.ToLower(entry)]++
	}
	for _, entry := range b {
		key := strings.ToLower(entry)
		if count, ok := seen[key]; !ok || count == 0 {
			return false
		}
		seen[key]--
	}
	return true
}

func jsonEqual(a, b domain.JSONMap) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}

func schemaEqual(a, b domain.TemplateSchema) bool {
	sa := sanitizeSchema(a)
	sb := sanitizeSchema(b)
	return stringSlicesEqual(sa.Required, sb.Required) && stringSlicesEqual(sa.Optional, sb.Optional)
}

func sanitizeSchema(schema domain.TemplateSchema) domain.TemplateSchema {
	if schema.IsZero() {
		return schema
	}
	return domain.TemplateSchema{
		Required: uniqueStrings(schema.Required),
		Optional: uniqueStrings(schema.Optional),
	}
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, val := range values {
		key := strings.TrimSpace(val)
		if key == "" {
			continue
		}
		key = strings.ToLower(key)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, val)
	}
	return result
}

func normalizeTemplate(t domain.NotificationTemplate) domain.NotificationTemplate {
	t.Code = strings.TrimSpace(t.Code)
	t.Channel = strings.TrimSpace(strings.ToLower(t.Channel))
	t.Locale = strings.TrimSpace(t.Locale)
	t.Subject = strings.TrimSpace(t.Subject)
	t.Body = strings.TrimSpace(t.Body)
	t.Format = strings.TrimSpace(t.Format)
	t.Schema = sanitizeSchema(t.Schema)
	t.Metadata = mergeJSON(t.Metadata, nil)
	return t
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]int, len(a))
	for _, v := range a {
		seen[strings.ToLower(strings.TrimSpace(v))]++
	}
	for _, v := range b {
		key := strings.ToLower(strings.TrimSpace(v))
		if count, ok := seen[key]; !ok || count == 0 {
			return false
		}
		seen[key]--
	}
	return true
}

func namespaced(namespace, code string) string {
	if namespace == "" {
		return code
	}
	if strings.HasPrefix(code, namespace+".") {
		return code
	}
	return namespace + "." + code
}

func normalizeChannels(channels []string) []string {
	unique := make(map[string]struct{}, len(channels))
	result := make([]string, 0, len(channels))
	for _, ch := range channels {
		chTrim := strings.TrimSpace(strings.ToLower(ch))
		if chTrim == "" {
			continue
		}
		if _, ok := unique[chTrim]; ok {
			continue
		}
		unique[chTrim] = struct{}{}
		result = append(result, chTrim)
	}
	return result
}

func templateKeysFor(tpls []domain.NotificationTemplate) domain.StringList {
	keys := make(domain.StringList, 0, len(tpls))
	for _, tpl := range tpls {
		keys = append(keys, tpl.Channel+":"+tpl.Code)
	}
	return keys
}

func filterTemplatesByChannels(tpls []domain.NotificationTemplate, channels []string) []domain.NotificationTemplate {
	if len(channels) == 0 {
		return tpls
	}
	chSet := make(map[string]struct{}, len(channels))
	for _, ch := range channels {
		chSet[strings.ToLower(ch)] = struct{}{}
	}
	out := make([]domain.NotificationTemplate, 0, len(tpls))
	for _, tpl := range tpls {
		if _, ok := chSet[strings.ToLower(tpl.Channel)]; ok {
			out = append(out, tpl)
		}
	}
	return out
}

func baseTemplateFor(tpls []domain.NotificationTemplate, channel string) domain.NotificationTemplate {
	for _, tpl := range tpls {
		if strings.EqualFold(tpl.Channel, channel) {
			return tpl
		}
	}
	return domain.NotificationTemplate{}
}

func emailTemplateFor(tpls []domain.NotificationTemplate) domain.NotificationTemplate {
	return baseTemplateFor(tpls, "email")
}

func inAppTemplateFor(tpls []domain.NotificationTemplate) domain.NotificationTemplate {
	return baseTemplateFor(tpls, "in-app")
}

func defaultValue(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func codeOrEmpty(tpl *domain.NotificationTemplate) string {
	if tpl == nil {
		return ""
	}
	return tpl.Code
}

func idOrEmpty(tpl *domain.NotificationTemplate) string {
	if tpl == nil {
		return ""
	}
	return tpl.ID.String()
}
