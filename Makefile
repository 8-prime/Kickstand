# Development wants two processes: Vite on 5173 serving the app with hot
# reload, and the Go server on 8080 owning /api. Vite proxies between them.
#
# A release is one binary with the built app inside it.

.PHONY: dev-api dev-web build clean test

dev-api:
	cd server && go run .

dev-web:
	pnpm dev

# Build the frontend into the server so `./server/server` serves everything.
build:
	pnpm build
	rm -rf server/web/dist
	cp -r dist server/web/dist
	cd server && go build -o server .
	@echo "built server/server — run it and open http://localhost:8080"

test:
	cd server && go test ./...
	pnpm exec tsc --noEmit

clean:
	rm -rf dist server/web/dist server/server
