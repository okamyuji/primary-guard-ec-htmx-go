# primary-guard-ec-htmx-go

Zenn 記事「[OpenAIの規模ではなくてもRDB設計はPrimaryを守る考え方でだいたい対応できる](https://zenn.dev/okamyuji/articles/mysql-read-replica-write-ahead-htmx-go)」のサンプル実装です。

MySQL の Primary と Read Replica の実レプリケーション環境で動く EC アプリを、Go の標準ライブラリと HTMX で組んでいます。記事に登場する判断軸 (Reader と Writer の分離、read-after-write、cache stampede 抑制、outbox、集計テーブル、縮退運転、Cookie セッションと CSRF) を、すべて動くコードと統合テストで再現しています。

## 動作要件

- Go 1.25 以上
- Docker と Docker Compose
- 品質ゲート実行用ツール `staticcheck` `golangci-lint v2` `gitleaks v8` `govulncheck` `pre-commit`

## ローカル起動手順

```sh
# 1. Primary と Replica を起動します
docker compose up -d mysql-primary mysql-replica

# 2. Replica にレプリケーション設定を投入します
docker compose run --rm repl-setup

# 3. レプリ状態を確認します (Replica_IO_Running と Replica_SQL_Running が Yes)
make repl-status

# 4. Primary にマイグレーションを適用します
DB_PRIMARY_DSN='appuser:apppass@tcp(127.0.0.1:3316)/primaryguard?parseTime=true&loc=UTC&charset=utf8mb4&collation=utf8mb4_0900_ai_ci&multiStatements=true' \
  make migrate

# 5. シードユーザーを投入します (admin@example.com / Admin12345 ほか 2 名)
DB_PRIMARY_DSN='appuser:apppass@tcp(127.0.0.1:3316)/primaryguard?parseTime=true&loc=UTC&charset=utf8mb4&collation=utf8mb4_0900_ai_ci' \
  make seed

# 6. アプリを起動してブラウザで開きます
DB_PRIMARY_DSN='appuser:apppass@tcp(127.0.0.1:3316)/primaryguard?parseTime=true&loc=UTC&charset=utf8mb4&collation=utf8mb4_0900_ai_ci&multiStatements=true' \
DB_REPLICA_DSN='appuser:apppass@tcp(127.0.0.1:3317)/primaryguard?parseTime=true&loc=UTC&charset=utf8mb4&collation=utf8mb4_0900_ai_ci' \
  make run

open http://localhost:8080/
```

## 主要画面と接続先

| メソッド | パス | 接続先 | 認証 |
|---|---|---|---|
| GET | `/` | Reader | 任意 |
| GET | `/products/{id}` | Reader | 任意 |
| GET | `/suggest` | Reader | 任意 |
| GET | `/cart` | Writer (Primary) | 一般 |
| POST | `/cart/add` `/cart/update` `/cart/remove` | Writer | 一般と CSRF |
| POST | `/orders` | Writer | 一般と CSRF |
| GET | `/orders/{id}` | Reader (read-after-write 中は Primary) | 一般 |
| GET | `/orders` | Reader | 一般 |
| GET | `/admin/report` | Reader (3 秒 timeout) | 管理者 |

## レプリケーションを目で確認するコマンド

```sh
docker compose exec mysql-primary mysql -uappuser -papppass primaryguard \
  -e "CREATE TABLE IF NOT EXISTS repl_demo (id INT PRIMARY KEY); INSERT INTO repl_demo VALUES (1);"
sleep 2
docker compose exec mysql-replica mysql -uappuser -papppass primaryguard \
  -e "SELECT * FROM repl_demo;"
```

Replica にも `id = 1` が見えればレプリケーションが動いています。

## 縮退運転を試す手順

Replica の SQL スレッドを止めると、ReplicaHealth が連続失敗で `ReplicaState.Trip()` を呼び、`/admin/report` は 503 を返し、商品一覧は Primary 経由で縮退表示になります。

```sh
docker compose exec mysql-replica mysql -uroot -prootpass -e 'STOP REPLICA;'
# しばらく待ってから /admin/report と / を開いて挙動を確認します

docker compose exec mysql-replica mysql -uroot -prootpass -e 'START REPLICA;'
# ReplicaHealth が回復を検知して通常運転に戻ります
```

## 品質ゲート

手動と pre-commit と CI のいずれからも `scripts/quality-gate.sh` を呼びます。

```sh
make gate              # 単体テストのみ実行します
make gate-integration  # testcontainers で Primary と Replica を起動し統合テストも実行します
```

スクリプトが順に通すステップは次のとおりです。

1. `gofmt` 差分検知
2. `go vet ./...`
3. `staticcheck ./...`
4. `golangci-lint run ./...`
5. `gitleaks detect`
6. `govulncheck ./...`
7. `go test ./... -race -shuffle=on -count=1 [-tags=integration]`
8. `go build ./...`

## 環境変数

| 変数 | 用途 | 既定値 |
|---|---|---|
| `APP_ADDR` | サーバの bind アドレス | `:8080` |
| `APP_SECURE_COOKIE` | Cookie の Secure 属性 | `0` |
| `APP_SESSION_TTL` | セッション有効期間 | `24h` |
| `APP_REPORT_TIMEOUT` | レポート timeout | `3s` |
| `APP_READ_AFTER_WRITE` | read-after-write window | `5s` |
| `APP_REPLICA_LAG_TRIP` | Replica trip しきい値 | `2s` |
| `APP_REPLICA_HEALTH_INTERVAL` | ReplicaHealth 監視周期 | `5s` |
| `APP_OUTBOX_INTERVAL` | worker のポーリング周期 | `1s` |
| `APP_OUTBOX_BATCH` | worker 1 回の処理件数 | `50` |
| `APP_MIGRATIONS_DIR` | migrations ディレクトリ | `migrations` |
| `DB_PRIMARY_DSN` | Primary 用 DSN (必須) | なし |
| `DB_REPLICA_DSN` | Replica 用 DSN (未設定時は Primary を流用します) | なし |

## ライセンス

本プロジェクトは MIT License で配布しています。詳細は同梱の `LICENSE` ファイルを参照してください。

## 参考

- [OpenAIの規模ではなくてもRDB設計はPrimaryを守る考え方でだいたい対応できる](https://zenn.dev/okamyuji/articles/mysql-read-replica-write-ahead-htmx-go)
- [MySQL 8.4 リファレンス レプリケーション](https://dev.mysql.com/doc/refman/8.4/en/replication.html)
- [okamyuji/slack-skeleton-go-htmx](https://github.com/okamyuji/slack-skeleton-go-htmx) の品質ゲート設定を手本にしています
