package handlers

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	"automation-developer-guide/src/config"
	"automation-developer-guide/src/database"
	"automation-developer-guide/src/models"
	"automation-developer-guide/src/utils"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/oauth2"
)

// HandleHealth serves as a health check for the application
func HandleHealth(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "ok", "service": "automation-developer-guide"})
}

// HandleMe returns the authenticated user's details in JSON format
func HandleMe(c *fiber.Ctx) error {
	// User is already authenticated via middleware
	username := c.Locals("username").(string)
	email, _ := c.Locals("email").(string)
	userID, _ := c.Locals("user_id").(string)
	avatarURL, _ := c.Locals("avatar_url").(string)
	firstName, _ := c.Locals("first_name").(string)
	lastName, _ := c.Locals("last_name").(string)
	return c.JSON(fiber.Map{
		"ok": true,
		"user": fiber.Map{
			"githubId":  userID,
			"username":  username,
			"email":     email,
			"avatarUrl": avatarURL,
			"firstName": firstName,
			"lastName":  lastName,
		},
	})
}

// HandleLogin initiates the OAuth flow
func HandleLogin(c *fiber.Ctx) error {
	// Prevent caching of the login initiation to ensure fresh state
	c.Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
	c.Set("Pragma", "no-cache")
	c.Set("Expires", "0")

	// 1. Generate a random state string to prevent CSRF
	state, err := utils.GenerateRandomState()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to generate state")
	}

	// 2. Save state in a short-lived cookie
	samesite, secure := getCookieSettings()

	c.Cookie(&fiber.Cookie{
		Name:     config.StateKey,
		Value:    state,
		Expires:  time.Now().Add(10 * time.Minute),
		HTTPOnly: true,
		Secure:   secure,
		SameSite: samesite,
		Path:     "/",
	})

	// 3. Redirect user to GitHub
	url := config.OauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline)
	return c.Redirect(url, fiber.StatusSeeOther)
}

// HandleCallback processes the response from GitHub
func HandleCallback(c *fiber.Ctx) error {
	samesite, secure := getCookieSettings()

	// 1. Validate State (CSRF Protection)
	storedState := c.Cookies(config.StateKey)
	if storedState == "" {
		return c.Status(fiber.StatusBadRequest).SendString("State cookie not found")
	}

	queryState := c.Query("state")
	if queryState != storedState {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid state parameter")
	}

	// Remove state from session after validation
	c.Cookie(&fiber.Cookie{
		Name:     config.StateKey,
		Value:    "",
		Expires:  time.Now().Add(-time.Hour),
		MaxAge:   -1,
		HTTPOnly: true,
		Secure:   secure,
		SameSite: samesite,
		Path:     "/",
	})

	// 2. Exchange authorization code for access token
	code := c.Query("code")
	token, err := config.OauthConfig.Exchange(context.Background(), code)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to exchange token: " + err.Error())
	}

	// 3. Fetch User Info from GitHub
	client := config.OauthConfig.Client(context.Background(), token)
	resp, err := client.Get("https://api.github.com/user")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to get user info: " + err.Error())
	}
	defer resp.Body.Close()

	// Use a temporary struct to decode GitHub response to avoid type mismatch
	// between GitHub's numeric "id" and MongoDB's ObjectID "id"
	var githubUser struct {
		Login     string `json:"login"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
		Email     string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&githubUser); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to parse user info")
	}

	user := models.User{
		Login:     githubUser.Login,
		Name:      githubUser.Name,
		AvatarURL: githubUser.AvatarURL,
		Email:     githubUser.Email,
	}

	// If email is empty, fetch it from the emails endpoint
	if user.Email == "" {
		emailResp, err := client.Get("https://api.github.com/user/emails")
		if err == nil {
			defer emailResp.Body.Close()
			var emails []struct {
				Email    string `json:"email"`
				Primary  bool   `json:"primary"`
				Verified bool   `json:"verified"`
			}
			if err := json.NewDecoder(emailResp.Body).Decode(&emails); err == nil {
				for _, e := range emails {
					if e.Primary && e.Verified {
						user.Email = e.Email
						break
					}
				}
			}
		}
	}

	// Store or Update User in MongoDB
	// Check existence using Email instead of Login (Username)
	filter := bson.M{"email": user.Email}
	update := bson.M{"$set": user}

	var existingUser models.User
	var userID string

	err = database.FindOne("users", filter, &existingUser)
	if err == mongo.ErrNoDocuments {
		// User does not exist, create new
		result, err := database.CreateOne("users", user)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to create user in db")
		}
		userID = result.InsertedID.(primitive.ObjectID).Hex()
	} else if err == nil {
		// User exists, update details
		if _, err := database.UpdateOne("users", filter, update); err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to update user in db")
		}
		userID = existingUser.ID.Hex()
	} else {
		return c.Status(fiber.StatusInternalServerError).SendString("Database error: " + err.Error())
	}

	// 4. Generate JWT
	firstName, lastName := splitName(user.Name)
	jwtToken, err := utils.GenerateJWT(userID, user.Email, user.Login, user.AvatarURL, firstName, lastName)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to generate token")
	}

	// 5. Generate and store an exchange code for secure token relay
	exchangeCode, err := utils.GenerateRandomState()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to generate exchange code")
	}

	codeData := models.ExchangeCode{
		Code:      exchangeCode,
		JWTToken:  jwtToken,
		CreatedAt: time.Now(),
	}

	if _, err := database.CreateOne("exchange_codes", codeData); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to store exchange code")
	}

	// 6. Redirect with exchange code in query parameter
	redirectURL := config.ClientURL
	if strings.Contains(redirectURL, "?") {
		redirectURL += "&code=" + exchangeCode
	} else {
		redirectURL += "?code=" + exchangeCode
	}

	return c.Redirect(redirectURL, fiber.StatusSeeOther)
}

// HandleExchangeToken exchanges a short-lived code for a JWT token
func HandleExchangeToken(c *fiber.Ctx) error {
	var body struct {
		Code string `json:"code"`
	}

	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if body.Code == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "code is required"})
	}

	// 1. Find the exchange code in DB
	filter := bson.M{"code": body.Code}
	var codeData models.ExchangeCode

	err := database.FindOne("exchange_codes", filter, &codeData)
	if err == mongo.ErrNoDocuments {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid or expired code"})
	} else if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}

	// 2. Check for expiration (e.g., 5 minutes)
	if time.Since(codeData.CreatedAt) > 5*time.Minute {
		database.DeleteOne("exchange_codes", filter)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "code has expired"})
	}

	// 3. Success! Return the JWT and delete the exchange code (one-time use)
	defer database.DeleteOne("exchange_codes", filter)

	return c.JSON(fiber.Map{
		"ok":    true,
		"token": codeData.JWTToken,
	})
}


func splitName(name string) (string, string) {
	parts := strings.SplitN(strings.TrimSpace(name), " ", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return name, ""
}

func getCookieSettings() (string, bool) {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("ENV")), "production") {
		return "None", true
	}

	return "Lax", false
}
