# ONDC Developer Guide – Backend

Go backend for the ONDC developer guide app. Built with [Fiber](https://gofiber.io/), MongoDB, JWT auth, and OAuth2.

## Prerequisites

- Go 1.25+
- MongoDB (local or remote)
- OAuth2 client credentials for Github login

## Setup

1. **Clone and enter the repo**

   ```bash
   cd developer-guide
   ```

2. **Configure environment**

   Copy `.env.example` to `.env` (or create `.env`) and set:

   | Variable     | Description                       | Default (dev)               |
   | ------------ | --------------------------------- | --------------------------- |
   | `ENV`        | `development` or `production`     | `development`               |
   | `PORT`       | HTTP server port                  | `8080`                      |
   | `MONGO_URI`  | MongoDB connection string         | `mongodb://localhost:27017` |
   | `DB_NAME`    | MongoDB database name             | `developer_guide_db`        |
   | `JWT_SECRET` | Secret for signing JWTs           | —                           |
   | `CLIENT_URL` | Allowed CORS origin (frontend)    | `http://localhost:5173`     |
   | OAuth2 vars  | Client ID,Secret and Redirect URL | —                           |

3. **Run the server**

   ```bash
   go run main.go
   ```

   Server listens at `http://localhost:8080` (or the port you set in `PORT`).

## Project layout

- `main.go` – Entry point, Fiber app, CORS, route setup
- `src/config/` – Config loading (e.g. from `.env`)
- `src/database/` – MongoDB connection
- `src/handlers/` – Auth (OAuth2 & Token Exchange), notes, comments handlers
- `src/middleware/` – Auth middleware (Bearer Token validation)
- `src/models/` – User, note, comment, and exchange code models
- `src/routes/` – Route registration
- `src/utils/` – JWT, random state, and crypto helpers

## Authentication Flow

This project uses a **Secure Token Exchange Flow** for cross-domain authentication:

1. **OAuth Redirect:** Backend redirects to frontend with a short-lived `code`.
2. **Token Exchange:** Frontend calls `POST /auth/exchange` with the `code` to get a JWT.
3. **Bearer Auth:** Frontend sends the JWT in the `Authorization: Bearer <token>` header for all subsequent requests.

Cookies are not used for authorization, making the backend fully cross-origin compatible.

## License

See repository license.   
