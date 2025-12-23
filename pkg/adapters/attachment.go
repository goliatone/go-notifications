package adapters

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"
)

// Attachment captures raw file payloads or URL references for adapters that support attachments.
type Attachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type,omitempty"`
	Content     []byte `json:"content"`
	Size        int    `json:"size,omitempty"`
	URL         string `json:"url,omitempty"`
}

// NormalizeAttachments sanitizes attachment slices and fills missing sizes.
func NormalizeAttachments(attachments []Attachment) []Attachment {
	if len(attachments) == 0 {
		return nil
	}
	out := make([]Attachment, 0, len(attachments))
	for _, att := range attachments {
		att.Filename = strings.TrimSpace(att.Filename)
		att.ContentType = strings.TrimSpace(att.ContentType)
		att.URL = strings.TrimSpace(att.URL)
		hasContent := len(att.Content) > 0
		hasURL := att.URL != ""
		if att.Filename == "" && hasURL {
			att.Filename = filenameFromURL(att.URL)
		}
		if att.Filename == "" && hasURL {
			att.Filename = "attachment"
		}
		if att.Filename == "" || (!hasContent && !hasURL) {
			continue
		}
		if att.ContentType == "" {
			att.ContentType = "application/octet-stream"
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
		URL:         stringValue(item, "url"),
	}
}

func attachmentFromStringMap(item map[string]string) Attachment {
	return Attachment{
		Filename:    strings.TrimSpace(item["filename"]),
		ContentType: strings.TrimSpace(item["content_type"]),
		Content:     []byte(item["content"]),
		URL:         strings.TrimSpace(item["url"]),
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

func filenameFromURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	base := path.Base(parsed.Path)
	if base == "." || base == "/" {
		return ""
	}
	return strings.TrimSpace(base)
}

// EmailAttachments filters attachments to those that include raw content.
func EmailAttachments(attachments []Attachment) []Attachment {
	if len(attachments) == 0 {
		return nil
	}
	normalized := NormalizeAttachments(attachments)
	if len(normalized) == 0 {
		return nil
	}
	out := make([]Attachment, 0, len(normalized))
	for _, att := range normalized {
		if len(att.Content) == 0 {
			continue
		}
		out = append(out, att)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// AttachmentURLs extracts URL strings from attachments.
func AttachmentURLs(attachments []Attachment) []string {
	if len(attachments) == 0 {
		return nil
	}
	normalized := NormalizeAttachments(attachments)
	if len(normalized) == 0 {
		return nil
	}
	out := make([]string, 0, len(normalized))
	for _, att := range normalized {
		if att.URL == "" {
			continue
		}
		out = append(out, att.URL)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ChannelAttachmentsFromValue normalizes channel attachment maps into a slice per channel.
func ChannelAttachmentsFromValue(value any) map[string][]Attachment {
	if value == nil {
		return nil
	}
	out := map[string][]Attachment{}
	switch v := value.(type) {
	case map[string][]Attachment:
		for key, list := range v {
			attachments := NormalizeAttachments(list)
			if len(attachments) == 0 {
				continue
			}
			out[strings.ToLower(strings.TrimSpace(key))] = attachments
		}
	case map[string]any:
		for key, entry := range v {
			attachments := AttachmentsFromValue(entry)
			if len(attachments) == 0 {
				continue
			}
			out[strings.ToLower(strings.TrimSpace(key))] = attachments
		}
	case map[string][]map[string]any:
		for key, list := range v {
			attachments := NormalizeAttachments(mapAttachments(list))
			if len(attachments) == 0 {
				continue
			}
			out[strings.ToLower(strings.TrimSpace(key))] = attachments
		}
	case map[string][]any:
		for key, list := range v {
			attachments := NormalizeAttachments(anyAttachments(list))
			if len(attachments) == 0 {
				continue
			}
			out[strings.ToLower(strings.TrimSpace(key))] = attachments
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ChannelAttachmentsFor returns channel-specific attachments when provided.
func ChannelAttachmentsFor(channelAttachments map[string][]Attachment, channel string) []Attachment {
	if len(channelAttachments) == 0 {
		return nil
	}
	key := strings.ToLower(strings.TrimSpace(channel))
	if key == "" {
		return nil
	}
	if attachments, ok := channelAttachments[key]; ok {
		return attachments
	}
	return nil
}
