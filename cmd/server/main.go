// Command server runs the API locally: `go run ./cmd/server`.
// It loads variables from .env if present.
package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"

	"aurabackend/pkg/app"
)

func main() {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Printf("warning: could not load .env: %v", err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("listening on http://localhost:%s", port)
	log.Fatal(srv.ListenAndServe())
}
