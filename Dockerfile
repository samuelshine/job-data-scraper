# ──────────────────────────────────────────────
# Stage 1: Build frontend
# ──────────────────────────────────────────────
FROM node:20-alpine AS frontend-builder

WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm install
COPY frontend/ ./
RUN npm run build

# ──────────────────────────────────────────────
# Stage 2: Build Go backend
# ──────────────────────────────────────────────
FROM golang:1.25-alpine AS backend-builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/

# Build static binary (no CGO — using modernc.org/sqlite)
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /server ./cmd/server

# ──────────────────────────────────────────────
# Stage 3: Final minimal runtime
# ──────────────────────────────────────────────
FROM node:20-alpine

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy built binary
COPY --from=backend-builder /server .

# Copy built frontend (for potential static serving)
COPY --from=frontend-builder /app/frontend/dist ./frontend/dist
COPY --from=frontend-builder /app/frontend/node_modules ./frontend/node_modules
COPY start.sh ./start.sh

# Create data directory for SQLite
RUN mkdir -p /data
RUN chmod +x /app/start.sh

ENV PORT=8080
ENV FRONTEND_PORT=4321
ENV DATABASE_PATH=/data/jobpulse.db

EXPOSE 8080

CMD ["/app/start.sh"]
