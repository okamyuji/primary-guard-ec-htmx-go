# primary-guard-ec-htmx-go

Zenn 記事「OpenAI規模でなくてもRDB設計はPrimaryを守る考え方でだいたい対応できる」の判断軸を、MySQL Primary と Read Replica の実レプリケーション環境で動く Go と HTMX の EC アプリとして写経した教材プロジェクトです。

参照記事のリンクは以下です。

- [OpenAI規模でなくてもRDB設計はPrimaryを守る考え方でだいたい対応できる](https://zenn.dev/okamyuji/articles/mysql-read-replica-write-ahead-htmx-go)

## なにを真似ているか

| 観点 | 本プロジェクトの実装 |
|---|---|
| read と write の分離 | `internal/dbx.DBRouter` で Writer と Reader を分け、書き込みは常に Primary、読み取りは原則 Replica に向ける |
| read-after-write | `dbx.WithReadAfterWrite(ctx, window)` で注文直後だけ ctx 単位に Primary を読む |
| cache stampede 抑制 | `internal/catalog.CategoryCache` で短 TTL + 同一キーの再取得を 1 回に集約 |
| outbox | 注文確定と同じ Tx で `outbox_events` に INSERT、cmd/worker が `FOR UPDATE SKIP LOCKED` で処理 |
| 集計テーブル | 注文確定と同じ Tx で `daily_sales_summary` を UPSERT、管理画面は Replica と集計テーブルだけ読む |
| 高コスト処理の保護 | `/admin/report` に context timeout (既定 3 秒) |
| 縮退運転 | `internal/dbx.ReplicaState` を Replica 遅延しきい値で Trip し、`/admin/report` を 503、`/suggest` を 503、商品一覧をキャッシュ寄りで返す |
| connection pool | `dbx.ConfigurePool` で MaxOpenConns 50、MaxIdleConns 25 などを明示 |
| 認証 | Cookie セッション + Go 1.24 標準入りの `crypto/pbkdf2` + Double Submit Cookie 型 CSRF |

## 動作要件

- Go 1.25 以上
- Docker (compose と統合テスト)
- 開発支援ツール (品質ゲート用)
  - `staticcheck` (`go install honnef.co/go/tools/cmd/staticcheck@latest`)
  - `golangci-lint` (`brew install golangci-lint` 等、v2 系)
  - `gitleaks` (`brew install gitleaks` 等、v8 系)
  - `govulncheck` (`go install golang.org/x/vuln/cmd/govulncheck@latest`)
  - `pre-commit` (`pipx install pre-commit` 等)

## ディレクトリ構成

```text
primary-guard-ec-htmx-go/
├── cmd/
│   ├── server/   HTTP サーバ起動エントリ
│   ├── worker/   outbox ワーカー起動エントリ
│   └── seed/     開発用ユーザーシード
├── internal/
│   ├── auth/        pbkdf2 / セッション / CSRF / middleware / DBSessionStore
│   ├── cart/        カート Repository
│   ├── catalog/     商品 Repository と CategoryCache
│   ├── config/      環境変数読み込み
│   ├── dbx/         DBRouter / Pool / ReplicaState / ReplicaHealth
│   ├── degrade/     Replica 障害時の縮退運転判定
│   ├── domain/      値型
│   ├── inventory/   在庫引当
│   ├── migrate/     migrations/*.up.sql の最小マイグレータ
│   ├── order/       注文 Repository (注文確定の 1 Tx 内で outbox と集計テーブルも更新)
│   ├── outbox/      Store と Worker
│   ├── render/      html/template と embed.FS のレンダラ
│   ├── report/      管理画面レポート Reader
│   ├── static/      embed.FS で同梱する styles.css と htmx.min.js
│   ├── transport/   PageData / middleware / 各 handler / Server 組み立て
│   └── user/        ユーザー Repository (UserLookup を満たす)
├── migrations/      0001..0005 の DDL とシード SQL
├── deploy/mysql/    primary.cnf / replica.cnf / repl 用初期 SQL
├── scripts/         quality-gate.sh と pre-commit 用 hook
├── .github/workflows/  CI
└── compose.yml      MySQL Primary + Replica + app + worker + repl-setup
```

## ローカル起動手順

```sh
# 1. MySQL の Primary と Replica を起動 (3316 と 3317 にバインド)
docker compose up -d mysql-primary mysql-replica

# 2. Replica にレプリケーション設定を投入する oneshot コンテナ
docker compose run --rm repl-setup

# 3. レプリ状態を目で確認 (Replica_IO_Running と Replica_SQL_Running が Yes)
make repl-status

# 4. マイグレーション適用 (Primary に直接、Replica は binlog で同期)
DB_PRIMARY_DSN='appuser:apppass@tcp(127.0.0.1:3316)/primaryguard?parseTime=true&loc=UTC&charset=utf8mb4&collation=utf8mb4_0900_ai_ci&multiStatements=true' \
  make migrate

# 5. シードユーザー投入 (admin@example.com / Admin12345 ほか 2 名)
DB_PRIMARY_DSN='appuser:apppass@tcp(127.0.0.1:3316)/primaryguard?parseTime=true&loc=UTC&charset=utf8mb4&collation=utf8mb4_0900_ai_ci' \
  make seed

# 6. サーバを起動してブラウザで開く
DB_PRIMARY_DSN='appuser:apppass@tcp(127.0.0.1:3316)/primaryguard?parseTime=true&loc=UTC&charset=utf8mb4&collation=utf8mb4_0900_ai_ci&multiStatements=true' \
DB_REPLICA_DSN='appuser:apppass@tcp(127.0.0.1:3317)/primaryguard?parseTime=true&loc=UTC&charset=utf8mb4&collation=utf8mb4_0900_ai_ci' \
  make run
open http://localhost:8080/
```

## 画面とアプリ内の接続先一覧

| メソッド | パス | 接続先 | 認証 | 説明 |
|---|---|---|---|---|
| GET | `/healthz` | なし | なし | ヘルスチェック |
| GET | `/` | Reader | 任意 | 商品一覧 |
| GET | `/products/{id}` | Reader | 任意 | 商品詳細 |
| GET | `/suggest` | Reader | 任意 | 検索サジェスト (Replica 障害時 503) |
| GET | `/login` | なし | なし | ログインフォーム |
| POST | `/login` | Writer | なし | ログイン処理 |
| GET | `/register` | なし | なし | 新規登録フォーム |
| POST | `/register` | Writer | なし | 新規登録処理 |
| POST | `/logout` | Writer | 任意 | ログアウト |
| GET | `/admin/login` | なし | なし | 管理者ログインフォーム |
| POST | `/admin/login` | Writer | なし | 管理者ログイン処理 |
| GET | `/cart` | Writer (自分の最新を見るため) | 一般 | カート画面 |
| POST | `/cart/add` | Writer | 一般 + CSRF | カート追加 |
| POST | `/cart/update` | Writer | 一般 + CSRF | カート数量更新 (HTMX) |
| POST | `/cart/remove` | Writer | 一般 + CSRF | カート行削除 (HTMX) |
| POST | `/orders` | Writer | 一般 + CSRF | 注文確定 + outbox + 集計 + Cart クリア (1 Tx) |
| GET | `/orders/{id}` | Reader (`?fresh=1` で RAW window 中は Primary) | 一般 | 注文詳細 |
| GET | `/orders` | Reader | 一般 | 注文履歴 |
| GET | `/admin/report` | Reader (Replica 障害時 503) | 管理者 | 月次売上レポート (3 秒 timeout) |

## レプリケーションを目で確認するコマンド

```sh
docker compose exec mysql-primary mysql -uappuser -papppass primaryguard \
  -e "CREATE TABLE IF NOT EXISTS repl_demo (id INT PRIMARY KEY); INSERT INTO repl_demo VALUES (1);"
sleep 2
docker compose exec mysql-replica mysql -uappuser -papppass primaryguard \
  -e "SELECT * FROM repl_demo;"
```

Replica にも `id = 1` が見えれば、binlog 経由のレプリケーションが動作しています。

## 縮退運転を試す

```sh
# Replica を止める (Replica_SQL_Running が No になる)
docker compose exec mysql-replica mysql -uroot -prootpass -e 'STOP REPLICA;'
```

`internal/dbx.ReplicaHealth` が `Seconds_Behind_Source` 取得失敗または閾値超過を検知して `ReplicaState.Trip()` し、`/admin/report` は 503、`/suggest` は 503、商品一覧は Primary 経由かつページ件数縮小になります。

復旧手順は次の通りです。

```sh
docker compose exec mysql-replica mysql -uroot -prootpass -e 'START REPLICA;'
```

しばらくすると ReplicaHealth が `ReplicaState.Reset()` を呼び、通常運転に戻ります。

## 品質ゲート

手動、`pre-commit`、CI から同じ `scripts/quality-gate.sh` を呼びます。

```sh
make gate              # 単体テストのみ
make gate-integration  # testcontainers で Primary+Replica を起動し統合テストも実行
```

スクリプトが順に通すステップは次の通りです。

1. `gofmt` 差分検知
2. `go vet ./...`
3. `staticcheck ./...`
4. `golangci-lint run ./...`
5. `gitleaks detect`
6. `govulncheck ./...`
7. `go test ./... -race -shuffle=on -count=1 [-tags=integration]`
8. `go build ./...`

`pre-commit` には同じ内容がフックとして登録されており、コミット時にローカルで自動実行します。CI は同じスクリプトを Ubuntu runner で呼び出します。

## 認証と CSRF

- パスワードは Go 1.24 で標準入りした `crypto/pbkdf2` を SHA-256 + 16 byte salt + 60 万回反復で扱います。比較は `crypto/subtle.ConstantTimeCompare`。
- セッション ID は `crypto/rand` で 32 byte 生成し、base64url で 43 文字。`sessions` テーブルに保存し、Cookie 属性は `Path=/; HttpOnly; SameSite=Lax`、本番では `APP_SECURE_COOKIE=1` で `Secure` を付けます。
- CSRF は Double Submit Cookie 方式で、Cookie とフォーム入力の値を `subtle.ConstantTimeCompare` で比較します。
- 一般画面と管理画面は同じセッション・CSRF 仕組みですが、`/admin/*` 配下は `auth.RequireAdmin` で `is_admin = 1` のユーザーだけが通過できます。

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
| `DB_REPLICA_DSN` | Replica 用 DSN (必須) | なし |

## 統合テストの仕組み

`make gate-integration` を実行すると、`internal/dbx` 配下の integration テストが testcontainers-go で次の手順を行います。

1. `mysql:8.4` を Primary として起動 (binlog ROW + GTID + log_bin)
2. `mysql:8.4` を Replica として起動 (read-only + GTID)
3. Primary に `repl` ユーザーを作成
4. Replica で `CHANGE REPLICATION SOURCE TO ... ; START REPLICA;`
5. `Replica_IO_Running` と `Replica_SQL_Running` が `Yes` になるのを待つ
6. Primary 経由で書いたデータが Replica で読めることを検証
7. `dbx.WithReadAfterWrite` で Reader が Primary を返すことを検証

## ライセンス

MIT License。詳しくは `LICENSE` を参照してください。

## 参考

- [OpenAI規模でなくてもRDB設計はPrimaryを守る考え方でだいたい対応できる](https://zenn.dev/okamyuji/articles/mysql-read-replica-write-ahead-htmx-go)
- [MySQL 8.4 リファレンス: レプリケーション](https://dev.mysql.com/doc/refman/8.4/en/replication.html)
- 同著者の [slack-skeleton-go-htmx](https://github.com/okamyuji/slack-skeleton-go-htmx) を品質ゲートのお手本にしています
