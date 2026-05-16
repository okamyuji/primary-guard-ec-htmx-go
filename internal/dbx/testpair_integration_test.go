//go:build integration

package dbx_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

// ReplicaPair Primary+Replica の MySQL 2 コンテナを束ねる
type ReplicaPair struct {
	Primary *sql.DB
	Replica *sql.DB
	Stop    func()
}

// StartReplicaPair Primary と Replica を起動し、レプリケーションを設定して返す
// testcontainers-go の network で同一ネットワークに乗せる
func StartReplicaPair(t *testing.T) *ReplicaPair {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	net, err := network.New(ctx)
	if err != nil {
		t.Fatalf("network new: %v", err)
	}

	primaryEnv := map[string]string{
		"MYSQL_ROOT_PASSWORD": "rootpass",
		"MYSQL_DATABASE":      "primaryguard",
		"MYSQL_USER":          "appuser",
		"MYSQL_PASSWORD":      "apppass",
	}
	primaryCmd := []string{
		"--server-id=1",
		"--log-bin=mysql-bin",
		"--binlog-format=ROW",
		"--gtid-mode=ON",
		"--enforce-gtid-consistency=ON",
		"--character-set-server=utf8mb4",
		"--collation-server=utf8mb4_0900_ai_ci",
	}
	primaryContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "mysql:8.4",
			Env:          primaryEnv,
			Cmd:          primaryCmd,
			ExposedPorts: []string{"3306/tcp"},
			Networks:     []string{net.Name},
			NetworkAliases: map[string][]string{
				net.Name: {"mysql-primary-it"},
			},
			WaitingFor: wait.ForLog("ready for connections").WithOccurrence(2).WithStartupTimeout(3 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("primary start: %v", err)
	}

	replicaEnv := map[string]string{
		"MYSQL_ROOT_PASSWORD": "rootpass",
	}
	replicaCmd := []string{
		"--server-id=2",
		"--relay-log=mysql-relay",
		"--read-only=ON",
		"--gtid-mode=ON",
		"--enforce-gtid-consistency=ON",
		"--character-set-server=utf8mb4",
		"--collation-server=utf8mb4_0900_ai_ci",
	}
	replicaContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "mysql:8.4",
			Env:          replicaEnv,
			Cmd:          replicaCmd,
			ExposedPorts: []string{"3306/tcp"},
			Networks:     []string{net.Name},
			NetworkAliases: map[string][]string{
				net.Name: {"mysql-replica-it"},
			},
			WaitingFor: wait.ForLog("ready for connections").WithOccurrence(2).WithStartupTimeout(3 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("replica start: %v", err)
	}

	primaryDB, err := openDB(ctx, primaryContainer, "root", "rootpass", "primaryguard")
	if err != nil {
		t.Fatalf("primary open: %v", err)
	}
	replicaRootDB, err := openDB(ctx, replicaContainer, "root", "rootpass", "mysql")
	if err != nil {
		t.Fatalf("replica root open: %v", err)
	}

	if _, err := primaryDB.ExecContext(ctx,
		"CREATE USER IF NOT EXISTS 'repl'@'%' IDENTIFIED WITH caching_sha2_password BY 'replpass'"); err != nil {
		t.Fatalf("create repl user: %v", err)
	}
	if _, err := primaryDB.ExecContext(ctx,
		"GRANT REPLICATION SLAVE ON *.* TO 'repl'@'%'"); err != nil {
		t.Fatalf("grant repl: %v", err)
	}

	const change = `CHANGE REPLICATION SOURCE TO
		SOURCE_HOST='mysql-primary-it',
		SOURCE_PORT=3306,
		SOURCE_USER='repl',
		SOURCE_PASSWORD='replpass',
		SOURCE_AUTO_POSITION=1,
		GET_SOURCE_PUBLIC_KEY=1`
	if _, err := replicaRootDB.ExecContext(ctx, change); err != nil {
		t.Fatalf("change source: %v", err)
	}
	if _, err := replicaRootDB.ExecContext(ctx, "START REPLICA"); err != nil {
		t.Fatalf("start replica: %v", err)
	}

	// レプリ確立待ち
	if err := waitForReplicaRunning(ctx, replicaRootDB); err != nil {
		t.Fatalf("wait replica: %v", err)
	}

	replicaAppDB, err := openDB(ctx, replicaContainer, "root", "rootpass", "primaryguard")
	if err != nil {
		t.Fatalf("replica root app db open: %v", err)
	}

	stop := func() {
		_ = primaryDB.Close()
		_ = replicaAppDB.Close()
		_ = replicaRootDB.Close()
		stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Minute)
		defer stopCancel()
		_ = replicaContainer.Terminate(stopCtx)
		_ = primaryContainer.Terminate(stopCtx)
		_ = net.Remove(stopCtx)
	}
	t.Cleanup(stop)

	return &ReplicaPair{Primary: primaryDB, Replica: replicaAppDB, Stop: stop}
}

// openDB ホスト経由でコンテナの MySQL に接続する
func openDB(ctx context.Context, c testcontainers.Container, user, pass, dbname string) (*sql.DB, error) {
	host, err := c.Host(ctx)
	if err != nil {
		return nil, err
	}
	port, err := c.MappedPort(ctx, "3306/tcp")
	if err != nil {
		return nil, err
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&loc=UTC&charset=utf8mb4&collation=utf8mb4_0900_ai_ci&multiStatements=true",
		user, pass, host, port.Port(), dbname)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	deadline, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	for {
		if err := db.PingContext(deadline); err == nil {
			return db, nil
		}
		select {
		case <-deadline.Done():
			return nil, fmt.Errorf("ping timeout: %w", deadline.Err())
		case <-time.After(time.Second):
		}
	}
}

// waitForReplicaRunning Replica の IO/SQL スレッドが Yes になるのを待つ
func waitForReplicaRunning(ctx context.Context, db *sql.DB) error {
	deadline, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	for {
		ok, err := isReplicaRunning(deadline, db)
		if err == nil && ok {
			return nil
		}
		select {
		case <-deadline.Done():
			return fmt.Errorf("replica not running: %w", deadline.Err())
		case <-time.After(time.Second):
		}
	}
}

// isReplicaRunning SHOW REPLICA STATUS の IO/SQL を確認する
func isReplicaRunning(ctx context.Context, db *sql.DB) (bool, error) {
	rows, err := db.QueryContext(ctx, "SHOW REPLICA STATUS")
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		return false, err
	}
	if !rows.Next() {
		return false, nil
	}
	values := make([]any, len(cols))
	holders := make([]any, len(cols))
	for i := range holders {
		holders[i] = &values[i]
	}
	if err := rows.Scan(holders...); err != nil {
		return false, err
	}
	io, sqlT := "", ""
	for i, c := range cols {
		switch c {
		case "Replica_IO_Running":
			io = toString(values[i])
		case "Replica_SQL_Running":
			sqlT = toString(values[i])
		}
	}
	return io == "Yes" && sqlT == "Yes", nil
}

// toString interface{} → string
func toString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	}
	return ""
}
