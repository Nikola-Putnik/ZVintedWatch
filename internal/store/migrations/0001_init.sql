-- Initial schema.

-- Free-form application settings stored as JSON values.
CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- Telegram chats (private chats, groups, channels) receiving notifications.
CREATE TABLE destinations (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT    NOT NULL,
    chat_id    TEXT    NOT NULL,
    created_at INTEGER NOT NULL
);

-- User-defined alerts. Filters are stored as JSON documents.
CREATE TABLE alerts (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    provider           TEXT    NOT NULL,
    name               TEXT    NOT NULL,
    enabled            INTEGER NOT NULL DEFAULT 1,
    search_input       TEXT    NOT NULL,
    domain             TEXT    NOT NULL DEFAULT '',
    title_filter       TEXT    NOT NULL DEFAULT '{}',
    description_filter TEXT    NOT NULL DEFAULT '{}',
    price_min          REAL,
    price_max          REAL,
    interval_min_sec   INTEGER NOT NULL,
    interval_max_sec   INTEGER NOT NULL,
    destination_ids    TEXT    NOT NULL DEFAULT '[]',
    created_at         INTEGER NOT NULL,
    updated_at         INTEGER NOT NULL
);
CREATE INDEX alerts_provider ON alerts (provider);

-- Polled searches, derived from alerts (one per distinct query).
CREATE TABLE searches (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    provider       TEXT    NOT NULL,
    key            TEXT    NOT NULL UNIQUE,
    label          TEXT    NOT NULL,
    web_url        TEXT    NOT NULL DEFAULT '',
    seeded         INTEGER NOT NULL DEFAULT 0,
    last_polled_at INTEGER NOT NULL DEFAULT 0,
    last_status    TEXT    NOT NULL DEFAULT '',
    last_error     TEXT    NOT NULL DEFAULT '',
    created_at     INTEGER NOT NULL
);

-- Item ids already returned by a search; used to detect new listings.
CREATE TABLE seen_items (
    search_id     INTEGER NOT NULL REFERENCES searches (id) ON DELETE CASCADE,
    item_id       TEXT    NOT NULL,
    first_seen_at INTEGER NOT NULL,
    last_seen_at  INTEGER NOT NULL,
    PRIMARY KEY (search_id, item_id)
) WITHOUT ROWID;
CREATE INDEX seen_items_time ON seen_items (last_seen_at);

-- Lazily fetched listing descriptions, shared by every alert.
CREATE TABLE item_cache (
    provider    TEXT    NOT NULL,
    item_id     TEXT    NOT NULL,
    description TEXT    NOT NULL,
    fetched_at  INTEGER NOT NULL,
    PRIMARY KEY (provider, item_id)
) WITHOUT ROWID;
CREATE INDEX item_cache_time ON item_cache (fetched_at);

-- Match history displayed in the UI.
CREATE TABLE matches (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    alert_id   INTEGER NOT NULL,
    alert_name TEXT    NOT NULL,
    provider   TEXT    NOT NULL,
    item_id    TEXT    NOT NULL,
    listing    TEXT    NOT NULL,
    matched_at INTEGER NOT NULL,
    delivery   TEXT    NOT NULL
);
CREATE UNIQUE INDEX matches_alert_item ON matches (alert_id, provider, item_id);
CREATE INDEX matches_time ON matches (matched_at);

-- Notifications already sent, so an item reaches a chat only once even when
-- several alerts or searches match it.
CREATE TABLE deliveries (
    destination_id INTEGER NOT NULL,
    provider       TEXT    NOT NULL,
    item_id        TEXT    NOT NULL,
    sent_at        INTEGER NOT NULL,
    PRIMARY KEY (destination_id, provider, item_id)
) WITHOUT ROWID;
CREATE INDEX deliveries_time ON deliveries (sent_at);
