package exportready

import "github.com/goliatone/go-notifications/pkg/domain"

// preparePayloadForChannel injects per-channel CTA/URL overrides into the payload.
// It mutates the provided map.
func preparePayloadForChannel(payload domain.JSONMap, channel string) {
	if payload == nil {
		return
	}
	overrides := channelOverrides(payload, channel)
	if overrides == nil {
		return
	}
	if label, ok := overrides["cta_label"].(string); ok && label != "" {
		payload["CTALabel"] = label
	}
	if link, ok := overrides["action_url"].(string); ok && link != "" {
		payload["ActionURL"] = link
	}
}

func channelOverrides(payload domain.JSONMap, channel string) map[string]any {
	if len(payload) == 0 {
		return nil
	}
	raw, ok := payload["channel_overrides"]
	if !ok {
		return nil
	}
	switch ov := raw.(type) {
	case map[string]any:
		if ch, ok := ov[channel]; ok {
			if m, ok := ch.(map[string]any); ok {
				return m
			}
		}
	case map[string]map[string]any:
		if m, ok := ov[channel]; ok {
			return m
		}
	}
	return nil
}
