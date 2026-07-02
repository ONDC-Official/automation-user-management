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
	api := app.Group("/", middleware.IsAuthenticated)

	// Auth
	api.Get("auth/api/me", handlers.HandleMe)

	// Comments
	api.Post("api/comments", handlers.HandleCreateComment)
	api.Put("api/comments/:id", handlers.HandleUpdateComment)
	api.Put("api/comments/:id/resolve", handlers.HandleResolveComment)
	api.Delete("api/comments/:id", handlers.HandleDeleteComment)

	// Notes
	api.Post("api/notes", handlers.HandleCreateNote)
	api.Put("api/notes/:id", handlers.HandleUpdateNote)
	api.Delete("api/notes/:id", handlers.HandleDeleteNote)

	// Scenario Preferences
	api.Get("user/scenario-preferences", handlers.HandleGetScenarioPreferences)
	api.Put("user/scenario-preferences/:config_key", handlers.HandleUpsertScenarioPreference)
	api.Delete("user/scenario-preferences/:config_key", handlers.HandleDeleteScenarioPreference)
}
