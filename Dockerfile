FROM node:22-alpine AS frontend-build
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.23-alpine AS backend-build
WORKDIR /src/backend
COPY backend/go.mod ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/pulsepoll .

FROM alpine:3.22
RUN addgroup -S pulsepoll && adduser -S pulsepoll -G pulsepoll
WORKDIR /app
COPY --from=backend-build /out/pulsepoll ./pulsepoll
COPY --from=frontend-build /src/frontend/dist ./frontend/dist
ENV STATIC_DIR=/app/frontend/dist
USER pulsepoll
EXPOSE 8080
CMD ["./pulsepoll"]
