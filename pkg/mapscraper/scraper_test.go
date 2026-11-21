package mapscraper

import (
	"context"
	"testing"
)

func TestScrapeLocationDynamicGoogleMaps(t *testing.T) {
	client := NewClient()
	ctx := context.Background()

	// Test Kawasan Marina / POJ Semarang coordinate
	res := client.ScrapeLocation(ctx, -6.9535, 110.3908, nil)
	if res.ZoneName == "" {
		t.Fatalf("expected non-empty scraped ZoneName, got empty")
	}
	if res.AQI <= 0 || res.PM25 <= 0 {
		t.Errorf("expected positive AQI and PM25, got AQI=%d PM25=%.1f", res.AQI, res.PM25)
	}
	t.Logf("Scraped POJ/Marina: name=%q category=%q aqi=%d pm25=%.1f source=%q",
		res.ZoneName, res.Category, res.AQI, res.PM25, res.Source)
}
