.PHONY: help build run test fmt vet lint gate gate-integration up down migrate repl-status

help:
	@echo "make build               アプリをビルドします"
	@echo "make run                 開発用にローカル起動します"
	@echo "make test                単体テストを race + shuffle + no-cache で実行します"
	@echo "make fmt                 go fmtで整形します"
	@echo "make vet                 go vetを走らせます"
	@echo "make lint                staticcheckとgolangci-lintを走らせます"
	@echo "make gate                品質ゲート(scripts/quality-gate.sh)を実行します"
	@echo "make gate-integration    統合テスト込みの品質ゲートを実行します"
	@echo "make up                  compose.ymlでMySQL Primary/Replicaとアプリを起動します"
	@echo "make down                composeで起動したスタックを停止します"
	@echo "make migrate             ローカルPrimary MySQLにマイグレーションを適用します"
	@echo "make repl-status         Replicaのレプリケーション状態を表示します"

build:
	go build -trimpath -o bin/server ./cmd/server
	go build -trimpath -o bin/worker ./cmd/worker

run:
	go run ./cmd/server

test:
	go test ./... -race -shuffle=on -count=1

fmt:
	gofmt -w .

vet:
	go vet ./...

lint:
	staticcheck ./...
	golangci-lint run ./...

gate:
	./scripts/quality-gate.sh

gate-integration:
	WITH_INTEGRATION=1 ./scripts/quality-gate.sh

up:
	docker compose up -d --build

down:
	docker compose down

migrate:
	@test -n "$$DB_PRIMARY_DSN" || (echo "DB_PRIMARY_DSN env が必要です" && exit 1)
	@for f in migrations/*.up.sql; do \
		echo "applying $$f"; \
		mysql --defaults-extra-file=<(printf "[client]\nuser=appuser\npassword=apppass\nhost=127.0.0.1\nport=3316\n") primaryguard < $$f || exit 1; \
	done

repl-status:
	docker compose exec mysql-replica mysql -uroot -prootpass -e 'SHOW REPLICA STATUS\G'
