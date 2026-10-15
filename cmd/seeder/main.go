// Command seeder scrapes and seeds hospital data into Firebase Firestore.
// Run with: `go run ./cmd/seeder`
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"

	"aurabackend/pkg/firebase"
	"aurabackend/pkg/hospitals"
)

func main() {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Printf("warning: could not load .env: %v", err)
	}

	googleAPIKey := os.Getenv("GOOGLE_PLACES_API_KEY")
	if googleAPIKey == "" {
		googleAPIKey = os.Getenv("FIREBASE_WEB_API_KEY")
	}

	var (
		apiKeyFlag   = flag.String("key", googleAPIKey, "Google Places API Key (or FIREBASE_WEB_API_KEY)")
		latFlag      = flag.Float64("lat", -6.9934, "Center latitude (Semarang by default)")
		lonFlag      = flag.Float64("lon", 110.4087, "Center longitude (Semarang by default)")
		radiusFlag   = flag.Float64("radius", 20000.0, "Search radius in meters")
		cityFlag     = flag.String("city", "Semarang", "City name for reference")
		dryRunFlag   = flag.Bool("dry-run", false, "Preview discovered hospitals without writing to Firestore")
	)
	flag.Parse()

	log.Println("==================================================")
	log.Println("       AURA Hospital Scraper & Seeder Tool        ")
	log.Println("==================================================")
	log.Printf("Target Center  : %.4f, %.4f (%s)", *latFlag, *lonFlag, *cityFlag)
	log.Printf("Search Radius  : %.1f km", *radiusFlag/1000.0)
	log.Printf("Dry Run Mode   : %v", *dryRunFlag)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	scraperCfg := hospitals.ScraperConfig{
		GoogleAPIKey: *apiKeyFlag,
		Latitude:     *latFlag,
		Longitude:    *lonFlag,
		RadiusMeters: *radiusFlag,
		City:         *cityFlag,
	}

	log.Println("\n[1/3] Scraping & Discovering Hospitals...")
	discovered := hospitals.DiscoverHospitals(ctx, scraperCfg)

	log.Printf("[2/3] Verification & Filtering completed: %d valid hospitals found", len(discovered))
	for i, h := range discovered {
		log.Printf("  [%2d] %-30s | ID: %-26s | Lat: %7.4f, Lon: %8.4f | Rating: %.1f (%d reviews)",
			i+1, h.Name, h.ID, h.Latitude, h.Longitude, h.Rating, h.UserRatingsTotal)
	}

	if *dryRunFlag {
		log.Println("\n[Dry Run] Skipped Firestore write. Exiting.")
		return
	}

	log.Println("\n[3/3] Initializing Firestore connection...")
	fbClients, err := firebase.Get(ctx)
	if err != nil {
		log.Fatalf("Fatal: failed to connect to Firebase/Firestore: %v", err)
	}

	store := hospitals.NewStore(fbClients.Firestore, fbClients.Config.HospitalsCollection)
	log.Printf("Writing %d hospitals into Firestore collection '%s'...", len(discovered), fbClients.Config.HospitalsCollection)

	savedCount, err := store.BatchUpsert(ctx, discovered)
	if err != nil {
		log.Fatalf("Fatal: failed to batch upsert into Firestore: %v", err)
	}

	log.Println("\n==================================================")
	log.Printf("SUCCESS: %d hospitals successfully saved to Firestore!", savedCount)
	log.Println("==================================================")
	fmt.Println("You can now test the endpoints:")
	fmt.Println("  1. Nearby: GET /api/v1/hospitals/nearby?lat=-6.9934&lon=110.4087&radius_km=15")
	fmt.Println("  2. Route : GET /api/v1/routes?origin_lat=-7.001692&origin_lon=110.41257&dest_lat=-6.9934&dest_lon=110.4087")
}
