CREATE TABLE github_events (
  id SERIAL PRIMARY KEY,
  payload jsonb NOT NULL DEFAULT '{}',
  pusher_name VARCHAR(255),
  pusher_email VARCHAR(255),
  commit_at timestamp DEFAULT current_timestamp,
  created_at timestamp DEFAULT current_timestamp,
  updated_at timestamp DEFAULT current_timestamp
);