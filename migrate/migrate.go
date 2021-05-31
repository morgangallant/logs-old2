// `migrate` migrates existing log messages from SQLite to PostgreSQL.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	logger "log"
	"time"

	"crawshaw.io/sqlite/sqlitex"
	_ "github.com/lib/pq"
)

var (
	sqlitePath  = flag.String("sqlite-path", "lp", "path to sqlite db")
	postgresUrl = flag.String("postgres-path", "pp", "postgres url")
)

func main() {
	flag.Parse()
	if err := run(); err != nil {
		logger.Fatal(err)
	}
}

type log struct {
	ts      time.Time
	content string
}

func existingLogs() ([]log, error) {
	pool, err := sqlitex.Open(*sqlitePath, 0, 10)
	if err != nil {
		return nil, err
	}
	defer pool.Close()
	conn := pool.Get(context.TODO())
	if conn == nil {
		return nil, errors.New("failed to get sqlite conn from pool")
	}
	defer pool.Put(conn)

	logs := []log{}
	// We order by ASC to insert them into the proper order into the Postgres DB.
	stmt := conn.Prep(`SELECT ts, content FROM logs ORDER BY datetime(ts) ASC;`)
	for {
		if hasNext, err := stmt.Step(); err != nil {
			return nil, err
		} else if !hasNext {
			break
		}
		ts, err := time.Parse(time.RFC3339, stmt.GetText("ts"))
		if err != nil {
			return nil, err
		}
		logs = append(logs, log{ts: ts, content: stmt.GetText("content")})
	}
	logger.Printf("Fetched %d logs from SQLite.", len(logs))
	return logs, nil
}

func migratePostgres(conn *sql.DB) error {
	stmt := `CREATE TABLE IF NOT EXISTS logs (id SERIAL PRIMARY KEY, timestamp TIMESTAMPTZ, content TEXT);`
	_, err := conn.Exec(stmt)
	return err
}

func insertLogs(logs []log) error {
	db, err := sql.Open("postgres", *postgresUrl)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		return err
	}
	if err := migratePostgres(db); err != nil {
		return err
	}
	stmt := `INSERT INTO logs (timestamp, content) VALUES ($1, $2);`
	for _, l := range logs {
		if _, err := db.Exec(stmt, l.ts, l.content); err != nil {
			return err
		}
	}
	logger.Printf("Inserted %d logs into PostgreSQL.", len(logs))
	return nil
}

func run() error {
	logs, err := existingLogs()
	if err != nil {
		return err
	}
	return insertLogs(logs)
}
