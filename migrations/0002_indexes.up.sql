-- 記事 5 章で示された商品一覧高速化用の複合インデックスを追加する
CREATE INDEX idx_products_status_updated ON products (status, updated_at, id);
CREATE INDEX idx_cart_items_user_updated ON cart_items (user_id, updated_at);
