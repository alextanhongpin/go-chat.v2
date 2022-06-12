start:
	@go run -race main.go

up:
	@docker-compose up -d

down:
	@docker-compose down
