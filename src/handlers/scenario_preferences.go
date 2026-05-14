package handlers

import (
	"log"
	"strings"

	"automation-developer-guide/src/database"
	"automation-developer-guide/src/models"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

var validateSP = validator.New()

func isValidConfigKey(key string) bool {
	return key != "" && !strings.Contains(key, "$") && !strings.ContainsRune(key, 0)
}

// HandleGetScenarioPreferences returns the full preferences map for the authenticated user.
func HandleGetScenarioPreferences(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(string)
	if !ok || userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var doc models.UserScenarioPreferences
	err := database.FindOne("scenario_preferences", bson.M{"user_id": userID}, &doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(fiber.Map{})
		}
		log.Printf("scenario_preferences GET error for user %s: %v", userID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to fetch preferences"})
	}

	result := make(map[string]models.PreferenceConfig, len(doc.Preferences))
	for _, entry := range doc.Preferences {
		result[entry.ConfigKey] = entry.PreferenceConfig
	}
	return c.JSON(result)
}

// HandleUpsertScenarioPreference upserts a single preference entry identified by config_key.
// config_key must equal domain_version_npType derived from the request body.
func HandleUpsertScenarioPreference(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(string)
	if !ok || userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	configKey := c.Params("config_key")
	if !isValidConfigKey(configKey) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid config_key"})
	}

	var config models.PreferenceConfig
	if err := c.BodyParser(&config); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if err := validateSP.Struct(config); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if config.NpType != "" && config.NpType != "BAP" && config.NpType != "BPP" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "np_type must be BAP or BPP"})
	}

	entry := models.PreferenceEntry{
		ConfigKey:        configKey,
		PreferenceConfig: config,
	}

	// Filter out any existing entry with the same key, then append the new one.
	// Using an array of entries (with "k" as the key field) avoids the MongoDB
	// restriction on dots in BSON field names (e.g. version "2.0.0").
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "user_id", Value: bson.D{{Key: "$ifNull", Value: bson.A{"$user_id", userID}}}},
			{Key: "preferences", Value: bson.D{
				{Key: "$concatArrays", Value: bson.A{
					bson.D{{Key: "$filter", Value: bson.D{
						{Key: "input", Value: bson.D{{Key: "$ifNull", Value: bson.A{"$preferences", bson.A{}}}}},
						{Key: "cond", Value: bson.D{{Key: "$ne", Value: bson.A{"$$this.k", configKey}}}},
					}}},
					bson.A{entry},
				}},
			}},
		}}},
	}

	filter := bson.M{"user_id": userID}
	if _, err := database.UpsertOne("scenario_preferences", filter, pipeline); err != nil {
		log.Printf("scenario_preferences PUT error for user %s key %s: %v", userID, configKey, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to save preference"})
	}

	return c.SendStatus(fiber.StatusOK)
}

// HandleDeleteScenarioPreference removes a single preference entry by config_key.
func HandleDeleteScenarioPreference(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(string)
	if !ok || userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	configKey := c.Params("config_key")
	if !isValidConfigKey(configKey) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid config_key"})
	}

	pipeline := mongo.Pipeline{
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "preferences", Value: bson.D{
				{Key: "$filter", Value: bson.D{
					{Key: "input", Value: bson.D{{Key: "$ifNull", Value: bson.A{"$preferences", bson.A{}}}}},
					{Key: "cond", Value: bson.D{{Key: "$ne", Value: bson.A{"$$this.k", configKey}}}},
				}},
			}},
		}}},
	}

	filter := bson.M{"user_id": userID}
	result, err := database.UpdateOne("scenario_preferences", filter, pipeline)
	if err != nil {
		log.Printf("scenario_preferences DELETE error for user %s key %s: %v", userID, configKey, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to delete preference"})
	}
	if result.MatchedCount == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "no preferences found"})
	}

	return c.SendStatus(fiber.StatusOK)
}
