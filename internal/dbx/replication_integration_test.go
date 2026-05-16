//go:build integration

package dbx_test

import (
	"context"
	"testing"
	"time"

	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/dbx"
)

// TestPrimaryWriteFlowsToReplica Primary に書き込んだ値が Replica で読めることを確認する
// 同時に DBRouter.Reader が forcePrimary でない場合に Replica を返すことも検証する
func TestPrimaryWriteFlowsToReplica(t *testing.T) {
	pair := StartReplicaPair(t)
	ctx := context.Background()

	if _, err := pair.Primary.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS repl_check (
		id BIGINT NOT NULL AUTO_INCREMENT, label VARCHAR(64) NOT NULL, PRIMARY KEY (id)) ENGINE=InnoDB`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := pair.Primary.ExecContext(ctx, "INSERT INTO repl_check (label) VALUES (?)", "from-primary"); err != nil {
		t.Fatalf("insert primary: %v", err)
	}

	router := dbx.New(pair.Primary, pair.Replica, dbx.NewReplicaState())
	reader := router.Reader(ctx)

	// レプリケーション反映待ち (最大 30 秒)
	deadline := time.Now().Add(30 * time.Second)
	var label string
	for {
		err := reader.QueryRowContext(ctx, "SELECT label FROM repl_check WHERE label = ?", "from-primary").Scan(&label)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("replica not catching up: %v", err)
		}
		time.Sleep(500 * time.Millisecond)
	}
	if label != "from-primary" {
		t.Fatalf("label got %s want from-primary", label)
	}

	// forcePrimary でない通常 read は Replica を選んでいる
	if router.Reader(ctx) != pair.Replica {
		t.Fatal("reader want replica")
	}
}

// TestForcePrimaryReadsPrimaryOnly read-after-write ctx で読むと Primary しか見ない
// Replica にだけ存在する行は読めず、Primary にだけ存在する行が読める
func TestForcePrimaryReadsPrimaryOnly(t *testing.T) {
	pair := StartReplicaPair(t)
	ctx := context.Background()

	if _, err := pair.Primary.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS only_check (
		id BIGINT NOT NULL AUTO_INCREMENT, label VARCHAR(64) NOT NULL, PRIMARY KEY (id)) ENGINE=InnoDB`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	// Primary に行を書く
	if _, err := pair.Primary.ExecContext(ctx, "INSERT INTO only_check (label) VALUES (?)", "primary-only"); err != nil {
		t.Fatalf("insert: %v", err)
	}

	router := dbx.New(pair.Primary, pair.Replica, dbx.NewReplicaState())
	ctxRAW := dbx.WithReadAfterWrite(ctx, 5*time.Second)
	if router.Reader(ctxRAW) != pair.Primary {
		t.Fatal("RAW ctx reader want primary")
	}
	var label string
	if err := router.Reader(ctxRAW).QueryRowContext(ctxRAW,
		"SELECT label FROM only_check WHERE label = ?", "primary-only").Scan(&label); err != nil {
		t.Fatalf("primary read: %v", err)
	}
	if label != "primary-only" {
		t.Fatalf("got %s want primary-only", label)
	}
}
