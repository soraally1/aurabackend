// Package handler is the Vercel Go serverless entrypoint. vercel.json rewrites
// requests to this function, and the router dispatches them.
package handler

import (
	"net/http"

	"aurabackend/pkg/app"
	"aurabackend/pkg/middleware"
)

// Handler is invoked by the Vercel Go runtime.
func Handler(w http.ResponseWriter, r *http.Request) {
	middleware.ResolvePath(r)
	app.Handler().ServeHTTP(w, r)
}

