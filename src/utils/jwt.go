package utils

import (
	"time"

	"automation-developer-guide/src/config"

	"github.com/golang-jwt/jwt/v5"
)

func GenerateJWT(userID, email, username, avatarURL, firstName, lastName string) (string, error) {
	claims := jwt.MapClaims{
		"user_id":    userID,
		"role":       "AUDITOR",
		"email":      email,
		"username":   username,
		"avatar_url": avatarURL,
		"first_name": firstName,
		"last_name":  lastName,
		"exp":        time.Now().Add(8 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.JWTSecret))
}

func ParseJWT(tokenString string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return []byte(config.JWTSecret), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, err
}
