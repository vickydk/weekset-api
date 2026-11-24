.PHONY: start stop

start:
	docker compose up -d --wait

stop:
	docker compose down
