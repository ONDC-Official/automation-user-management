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
   | `MCP_KEY_TTL_DAYS` | MCP API key lifetime in days. Must match section 3 of the MCP consent document | `90` |
   | `MCP_CONSENT_VERSION` | Consent version `POST /user/mcp-key` accepts | `2026-10-v1` |
   | `MCP_SERVICE_TOKEN` | Optional. When set, `/mcp/verify` also requires a matching `X-Service-Token` header | — (gate disabled) |

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
- `src/utils/` – JWT, random state, and crypto helpers (incl. MCP key generation and hashing)

## MCP API Keys

AI agents reach the Workbench MCP with a long-lived API key issued off the back of
GitHub login. The key is an opaque bearer secret of the form
`ondc_mcp_<43 url-safe base64 chars>` — **not** an asymmetric key. Only its SHA-256
hash is stored, so a lost key cannot be recovered, only replaced.

| Method | Path | Auth | Purpose |
| ------ | ---- | ---- | ------- |
| GET | `/user/mcp-key` | JWT | Key metadata (never the key). Returns `has_key:false` rather than 404 when there is none. |
| POST | `/user/mcp-key` | JWT | Issue a key, replacing any existing one. Body `{consent_version, consent_accepted}`. Returns the plaintext **once**. 409 on a stale `consent_version`. |
| DELETE | `/user/mcp-key` | JWT | Revoke. Idempotent. |
| POST | `/mcp/verify` | `X-Service-Token` (optional) | Called by the MCP server with `{key}`. Returns `{valid, user_id, username, email, expires_at}` or 401 `{valid:false, reason}`. |

One key per user, enforced by a unique index on `user_id`; regenerating overwrites it,
which is what makes the old key stop working immediately. Keys expire after
`MCP_KEY_TTL_DAYS`, enforced in the handlers — there is deliberately no TTL index, so
an expired key still reports as present and the consent record survives. See
`database.EnsureMCPKeyIndexes` for the full reasoning.

Callers of `/mcp/verify` must **fail closed**: treat any non-200, timeout or connection
error as a rejection, and do not cache the result.

## Authentication Flow

This project uses a **Secure Token Exchange Flow** for cross-domain authentication:

1. **OAuth Redirect:** Backend redirects to frontend with a short-lived `code`.
2. **Token Exchange:** Frontend calls `POST /auth/exchange` with the `code` to get a JWT.
3. **Bearer Auth:** Frontend sends the JWT in the `Authorization: Bearer <token>` header for all subsequent requests.

Cookies are not used for authorization, making the backend fully cross-origin compatible.

## License

See repository license.   
