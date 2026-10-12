// Package handler is the Vercel Go serverless entrypoint. vercel.json rewrites
// every request to this function, and the shared router dispatches it.
package handler

import (
	"net/http"

	"aurabackend/pkg/app"
)

// Handler is invoked by the Vercel Go runtime.
func Handler(w http.ResponseWriter, r *http.Request) {
	app.Handler().ServeHTTP(w, r)
}
