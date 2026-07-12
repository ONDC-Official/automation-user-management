package main

import (
	"log"
	"os"
	"strings"

	"automation-developer-guide/src/config"
	"automation-developer-guide/src/database"
	"automation-developer-guide/src/routes"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

func main() {
	// Initialize configuration
	config.Load()

	// Connect to MongoDB
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" && os.Getenv("ENV") == "development" {
		mongoURI = "mongodb://localhost:27017"
	}
	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		dbName = "developer_guide_db"
	}
	if err := database.Connect(mongoURI, dbName); err != nil {
		log.Fatal("Failed to connect to MongoDB:", err)
	}

	// 1.5 Ensure TTL index for exchange codes (expires after 5 minutes)
	if err := database.EnsureTTLIndex("exchange_codes", "created_at", 300); err != nil {
		log.Println("Warning: Failed to create TTL index for exchange_codes:", err)
	}

	if err := database.EnsureUniqueIndex("scenario_preferences", "user_id"); err != nil {
		log.Println("Warning: Failed to create unique index for scenario_preferences:", err)
	}
	
	if err := database.EnsureCommentIndexes(); err != nil {
		log.Println("Warning: Failed to create indexes for comments:", err)
	}

	app := fiber.New()

	// CORS: allow frontend origin (credentials for cookies/session)
	corsCfg := cors.Config{
		AllowOrigins:     config.ClientURL,
		AllowCredentials: true,
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization",
		// cookie fix
		AllowMethods:     "GET,POST,PUT,PATCH,DELETE,OPTIONS",
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("ENV")), "production") {
		corsCfg.AllowOrigins = "https://workbench.ondc.tech"
	}
	app.Use(cors.New(corsCfg))

	// Routes
	routes.Setup(app)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server running at http://localhost:%s", port)
	log.Fatal(app.Listen(":" + port))
}
