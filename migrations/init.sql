CREATE TABLE IF NOT EXISTS requests (
                                        id SERIAL PRIMARY KEY,
                                        method TEXT,
                                        url TEXT,
                                        headers JSONB
);

CREATE TABLE IF NOT EXISTS responses (
                                         id SERIAL PRIMARY KEY,
                                         request_id INTEGER REFERENCES requests(id),
                                         status INTEGER,
                                         headers JSONB,
                                         length BIGINT
)