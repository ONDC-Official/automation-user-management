package routes

import (
	"automation-developer-guide/src/handlers"
	"automation-developer-guide/src/middleware"

	"github.com/gofiber/fiber/v2"
)

func Setup(app *fiber.App) {
	// ─── Public Routes ───────────────────────────────────────────
	app.Get("/", handlers.HandleHealth)
	app.Get("/health", handlers.HandleHealth)
	app.Get("/login", handlers.HandleLogin)
	app.Get("/auth/github/callback", handlers.HandleCallback)
	app.Post("/auth/exchange", handlers.HandleExchangeToken)
	

	
	// Comments (Public)
	app.Get("api/comments", handlers.HandleGetComments)
	app.Get("api/comments/:id", handlers.HandleGetCommentByID)

	// Notes (Public)
	app.Get("api/notes", handlers.HandleGetNotes)

	// ─── Protected Routes (require authentication) ───────────────
	// Note: middleware applied per-route — app.Group("/", mw) in Fiber v2 is
	// equivalent to app.Use("/", mw) and would intercept /health.
	auth := middleware.IsAuthenticated

	// Auth
	app.Get("/auth/api/me", auth, handlers.HandleMe)

	// Comments
	app.Post("/api/comments", auth, handlers.HandleCreateComment)
	app.Put("/api/comments/:id", auth, handlers.HandleUpdateComment)
	app.Put("/api/comments/:id/resolve", auth, handlers.HandleResolveComment)
	app.Delete("/api/comments/:id", auth, handlers.HandleDeleteComment)

	// Notes
	app.Post("/api/notes", auth, handlers.HandleCreateNote)
	app.Put("/api/notes/:id", auth, handlers.HandleUpdateNote)
	app.Delete("/api/notes/:id", auth, handlers.HandleDeleteNote)

	// Scenario Preferences
	app.Get("/user/scenario-preferences", auth, handlers.HandleGetScenarioPreferences)
	app.Put("/user/scenario-preferences/:config_key", auth, handlers.HandleUpsertScenarioPreference)
	app.Delete("/user/scenario-preferences/:config_key", auth, handlers.HandleDeleteScenarioPreference)
}
