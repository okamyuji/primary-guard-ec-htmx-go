-- 管理画面レポート向けの日次集計テーブルを追加する
CREATE TABLE daily_sales_summary (
    sales_date DATE NOT NULL,
    total_orders INT NOT NULL,
    total_amount BIGINT NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    PRIMARY KEY (sales_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
