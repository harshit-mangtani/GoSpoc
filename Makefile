DB_URL=postgres://judge:judge@host.docker.internal:5432/judge?sslmode=disable

db-up:
	docker compose up -d

db-down:
	docker compose down

db-logs:
	docker compose logs -f postgres

migrate-up:
	docker run --rm -v "$(PWD)/migrations:/migrations" migrate/migrate -path=/migrations -database "$(DB_URL)" up

migrate-down:
	docker run --rm -v "$(PWD)/migrations:/migrations" migrate/migrate -path=/migrations -database "$(DB_URL)" down 1

migrate-version:
	docker run --rm -v "$(PWD)/migrations:/migrations" migrate/migrate -path=/migrations -database "$(DB_URL)" version
	
psql:
	docker compose exec postgres psql -U judge -d judge
