-- Highest listing sequence seen per search, used to ignore old listings that
-- resurface at the top of the results ("bumped" listings).
ALTER TABLE searches ADD COLUMN watermark INTEGER NOT NULL DEFAULT 0;
