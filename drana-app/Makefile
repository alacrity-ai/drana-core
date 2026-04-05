.PHONY: dev build preview lint docker install

install: ## Install dependencies
	npm install

dev: ## Start dev server with hot reload
	npm run dev

build: ## Build for production
	npm run build

preview: ## Preview production build locally
	npm run preview

lint: ## Type check
	npx tsc --noEmit

docker: ## Build Docker image
	docker build -t drana-app .
