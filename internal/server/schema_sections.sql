CREATE TABLE IF NOT EXISTS songbook_sections (
  id uuid PRIMARY KEY,
  songbook_id uuid NOT NULL REFERENCES songbooks (id) ON DELETE CASCADE,
  title text NOT NULL,
  position int NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS songbook_section_songs (
  section_id uuid NOT NULL REFERENCES songbook_sections (id) ON DELETE CASCADE,
  songbook_id uuid NOT NULL,
  fbid text NOT NULL,
  position int NOT NULL DEFAULT 0,
  PRIMARY KEY (section_id, songbook_id, fbid),
  FOREIGN KEY (songbook_id, fbid) REFERENCES songs (songbook_id, fbid) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS section_songs_song_idx ON songbook_section_songs (songbook_id, fbid);
