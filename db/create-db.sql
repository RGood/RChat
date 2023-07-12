CREATE TABLE IF NOT EXISTS users(
    username text PRIMARY KEY,
    password bytea NOT NULL,
    salt text NOT NULL
);
