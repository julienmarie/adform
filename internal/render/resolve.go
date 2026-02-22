package render

import (
	"strings"

	"adform/internal/config"
)

func Resolve(bundle config.Bundle) config.Bundle {
	resolved := bundle
	resolved.Ads = make(map[string]config.Ad, len(bundle.Ads))
	for key, ad := range bundle.Ads {
		adCopy := ad
		adset, ok := bundle.Adsets[ad.AdsetKey]
		if ok {
			campaign, ok := bundle.Campaigns[adset.CampaignKey]
			if ok {
				adCopy.Tracking.UTM.Campaign = strings.ReplaceAll(adCopy.Tracking.UTM.Campaign, "{{campaign.key}}", campaign.Key)
			}
		}
		creative, ok := bundle.Creatives[ad.CreativeKey]
		if ok {
			adCopy.Tracking.UTM.Content = strings.ReplaceAll(adCopy.Tracking.UTM.Content, "{{creative.key}}", creative.Key)
		}
		resolved.Ads[key] = adCopy
	}
	return resolved
}
