-- レプリケーション用ユーザーを作成する
CREATE USER IF NOT EXISTS 'repl'@'%' IDENTIFIED WITH caching_sha2_password BY 'replpass';
GRANT REPLICATION SLAVE ON *.* TO 'repl'@'%';

-- アプリ用ユーザーに ReplicaHealth が SHOW REPLICA STATUS を実行できる権限を与える
GRANT REPLICATION CLIENT ON *.* TO 'appuser'@'%';

FLUSH PRIVILEGES;
