package adapters

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

// Attachment captures raw file payloads for adapters that support attachments.
type Attachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type,omitempty"`
	Content     []byte `json:"content"`
	Size        int    `json:"size,omitempty"`
}

// NormalizeAttachments sanitizes attachment slices and fills missing sizes.
func NormalizeAttachments(attachments []Attachment) []Attachment {
	if len(attachments) == 0 {
		return nil
	}
	out := make([]Attachment, 0, len(attachments))
	for _, att := range attachments {
		att.Filename = strings.TrimSpace(att.Filename)
		if att.Filename == "" || len(att.Content) == 0 {
			continue
		}
		if att.Size == 0 {
			att.Size = len(att.Content)
		}
		out = append(out, att)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// AttachmentsFromValue normalizes the supported attachment payload shapes into a slice.
func AttachmentsFromValue(value any) []Attachment {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case []Attachment:
		return NormalizeAttachments(v)
	case Attachment:
		return NormalizeAttachments([]Attachment{v})
	case []map[string]any:
		return NormalizeAttachments(mapAttachments(v))
	case []any:
		return NormalizeAttachments(anyAttachments(v))
	default:
		return nil
	}
}

func mapAttachments(items []map[string]any) []Attachment {
	if len(items) == 0 {
		return nil
	}
	out := make([]Attachment, 0, len(items))
	for _, item := range items {
		att := attachmentFromMap(item)
		out = append(out, att)
	}
	return out
}

func anyAttachments(items []any) []Attachment {
	if len(items) == 0 {
		return nil
	}
	out := make([]Attachment, 0, len(items))
	for _, item := range items {
		switch entry := item.(type) {
		case Attachment:
			out = append(out, entry)
		case map[string]any:
			out = append(out, attachmentFromMap(entry))
		case map[string]string:
			out = append(out, attachmentFromStringMap(entry))
		default:
			continue
		}
	}
	return out
}

func attachmentFromMap(item map[string]any) Attachment {
	return Attachment{
		Filename:    stringValue(item, "filename", "name"),
		ContentType: stringValue(item, "content_type", "contentType", "type"),
		Content:     bytesValue(item, "content", "data"),
		Size:        intValue(item, "size"),
	}
}

func attachmentFromStringMap(item map[string]string) Attachment {
	return Attachment{
		Filename:    strings.TrimSpace(item["filename"]),
		ContentType: strings.TrimSpace(item["content_type"]),
		Content:     []byte(item["content"]),
	}
}

func stringValue(item map[string]any, keys ...string) string {
	for _, key := range keys {
		raw, ok := item[key]
		if !ok {
			continue
		}
		switch v := raw.(type) {
		case string:
			if s := strings.TrimSpace(v); s != "" {
				return s
			}
		default:
			if s := strings.TrimSpace(fmt.Sprint(v)); s != "" {
				return s
			}
		}
	}
	return ""
}

func bytesValue(item map[string]any, keys ...string) []byte {
	for _, key := range keys {
		raw, ok := item[key]
		if !ok || raw == nil {
			continue
		}
		switch v := raw.(type) {
		case []byte:
			if len(v) > 0 {
				return v
			}
		case string:
			if v == "" {
				continue
			}
			if decoded, err := base64.StdEncoding.DecodeString(v); err == nil {
				return decoded
			}
			return []byte(v)
		}
	}
	return nil
}

func intValue(item map[string]any, keys ...string) int {
	for _, key := range keys {
		raw, ok := item[key]
		if !ok || raw == nil {
			continue
		}
		switch v := raw.(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			return int(v)
		case string:
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
				return n
			}
		default:
			if n, err := strconv.Atoi(fmt.Sprint(v)); err == nil {
				return n
			}
		}
	}
	return 0
}
