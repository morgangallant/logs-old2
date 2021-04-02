package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"time"

	"crawshaw.io/sqlite"
	"crawshaw.io/sqlite/sqlitex"
)

var (
	key      string
	username string
)

func init() {
	var ok bool
	key, ok = os.LookupEnv("TELEGRAM_KEY")
	if !ok {
		panic("missing TELEGRAM_KEY environment variable")
	}
	username, ok = os.LookupEnv("TELEGRAM_USERNAME")
	if !ok {
		panic("missing TELEGRAM_USERNAME environment variable")
	}
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func migrate(conn *sqlite.Conn) (err error) {
	defer sqlitex.Save(conn)(&err)
	err = sqlitex.Exec(conn, `
		CREATE TABLE IF NOT EXISTS logs (
			ts DATETIME NOT NULL,
			content TEXT NOT NULL
		);`, nil)
	return
}

func insertLog(conn *sqlite.Conn, ts time.Time, content string) (err error) {
	defer sqlitex.Save(conn)(&err)
	err = sqlitex.Exec(conn, `INSERT INTO logs (
		ts, content
	) VALUES (
		?, ?
	);`, nil, ts.Format(time.RFC3339), content)
	return
}

func dbpath() string {
	// For local environments.
	if runtime.GOOS == "darwin" {
		return "mglogs.db"
	}
	return "/root/storage/mglogs.db"
}

const addr = ":11108"

var dbpool *sqlitex.Pool

func run() error {
	var err error
	dbpool, err = sqlitex.Open(dbpath(), 0, 10)
	if err != nil {
		return err
	}
	conn := dbpool.Get(context.TODO())
	if conn == nil {
		return errors.New("nil connection")
	}
	if err := migrate(conn); err != nil {
		return err
	}
	dbpool.Put(conn)
	log.Printf("Starting server.")
	http.HandleFunc("/", getHandler())
	http.HandleFunc("/_wh/telegram", telegramHandler())
	return http.ListenAndServe(addr, nil)
}

const currentTimezone = "America/Vancouver"

func tz() *time.Location {
	loc, err := time.LoadLocation(currentTimezone)
	if err != nil {
		panic(err)
	}
	return loc
}

func printHTMLHead(w io.Writer, title string) {
	fmt.Fprintln(w, "<head>")
	fmt.Fprintln(w, `<meta charset="UTF-8" />`)
	fmt.Fprintln(w, `<meta name="viewport" content="width=device-width, initial-scale=1.0" />`)
	fmt.Fprintf(w, "<title>%s</title>", title)
	fmt.Fprintln(w, "</head>")
}

const (
	dayFormat  = "2006-01-02"
	timeFormat = "15:04"
)

func getHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		conn := dbpool.Get(r.Context())
		if conn == nil {
			return
		}
		defer dbpool.Put(conn)
		loc := tz()
		fmt.Fprintln(w, `<html lang="en">`)
		printHTMLHead(w, "Morgan's Logs")
		fmt.Fprintln(w, "<body>")
		fmt.Fprintln(w, "<div style=\"max-width: 960px; margin: 0 auto;\">")
		fmt.Fprintln(w, "<p><strong>Morgan's Logs</strong></p>")
		fmt.Fprintf(w, "<p>Current TZ: %s.</p>\n", currentTimezone)
		fmt.Fprintln(w, "<ul>")
		stmt := conn.Prep(`SELECT ts, content FROM logs ORDER BY datetime(ts) DESC;`)
		var count, prevday int
		for {
			if hasNext, err := stmt.Step(); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			} else if !hasNext {
				break
			}
			ts, err := time.Parse(time.RFC3339, stmt.GetText("ts"))
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			ts = ts.In(loc)
			if day := ts.Day(); day != prevday {
				fmt.Fprintf(w, "<p>%s</p>\n", ts.Format(dayFormat))
				prevday = day
			}
			fmt.Fprintf(w, "<li>%s: %s</li>\n", ts.Format(timeFormat), stmt.GetText("content"))
			count++
		}
		fmt.Fprintln(w, "</ul>")
		fmt.Fprintf(w, "<p style=\"text-align: center;\">Rendered %d logs in %d ms.</p>", count, time.Since(start).Milliseconds())
		fmt.Fprintln(w, "</div>")
		fmt.Fprintln(w, "</body>")
		fmt.Fprintln(w, "</html>")
		w.Header().Set("Content-Type", "text/html")
	}
}

func telegramHandler() http.HandlerFunc {
	type chat struct {
		ID int `json:"id"`
	}
	type from struct {
		ID        int    `json:"id"`
		IsBot     bool   `json:"is_bot"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		Username  string `json:"username"`
	}
	type message struct {
		Text string `json:"text"`
		Chat chat   `json:"chat"`
		From from   `json:"from"`
	}
	type webhook struct {
		Message message `json:"message"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		conn := dbpool.Get(r.Context())
		if conn == nil {
			return
		}
		defer dbpool.Put(conn)
		if whkeys, ok := r.URL.Query()["key"]; !ok || len(whkeys) == 0 || whkeys[0] != key {
			http.Error(w, "invalid key", http.StatusUnauthorized)
			return
		}
		var wh webhook
		if err := json.NewDecoder(r.Body).Decode(&wh); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if wh.Message.From.Username != username {
			// Ignore.
			return
		}
		if err := insertLog(conn, time.Now(), wh.Message.Text); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}
