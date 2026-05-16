// Package domain アプリ全体で共有する値型を定義する
// 構造体は値オブジェクトとして扱い、SQL や HTTP の知識を持たない
package domain

import (
	"encoding/json"
	"time"
)

// Category 商品カテゴリ
type Category struct {
	ID        int64
	Name      string
	UpdatedAt time.Time
}

// Product 商品
type Product struct {
	ID         int64
	CategoryID int64
	Name       string
	PriceYen   int
	Status     string
	UpdatedAt  time.Time
}

// CartItem カート行
type CartItem struct {
	UserID    int64
	ProductID int64
	Quantity  int
	UpdatedAt time.Time
}

// CartRow ハンドラがテンプレートに渡すカート 1 行分
type CartRow struct {
	ProductID    int64
	Name         string
	Quantity     int
	UnitPriceYen int
	Subtotal     int
}

// Order 注文
type Order struct {
	ID        int64
	UserID    int64
	TotalYen  int
	Status    string
	CreatedAt time.Time
	Items     []OrderItem
}

// OrderItem 注文明細
type OrderItem struct {
	OrderID      int64
	ProductID    int64
	ProductName  string
	Quantity     int
	UnitPriceYen int
}

// Subtotal 明細小計を返す
func (oi OrderItem) Subtotal() int {
	return oi.Quantity * oi.UnitPriceYen
}

// User 利用者
type User struct {
	ID        int64
	Email     string
	IsAdmin   bool
	CreatedAt time.Time
}

// OutboxEvent 非同期に処理されるイベント
type OutboxEvent struct {
	ID          int64
	Type        string
	Payload     json.RawMessage
	ProcessedAt *time.Time
	CreatedAt   time.Time
}

// DailySalesSummary 日次売上集計
type DailySalesSummary struct {
	Date        time.Time
	TotalOrders int
	TotalAmount int
	UpdatedAt   time.Time
}
