package main

import (
	"database/sql"
	"encoding/gob"
	"fmt"
	"github.com/alexedwards/scs/redisstore"
	"github.com/alexedwards/scs/v2"
	"github.com/gomodule/redigo/redis"
	_ "github.com/jackc/pgconn"
	_ "github.com/jackc/pgx/v4"
	_ "github.com/jackc/pgx/v4/stdlib"
	"log"
	"net/http"
	"os"
	"os/signal"
	"subscription-service/data"
	"sync"
	"syscall"
	"time"
)

const webPort = "80"

func main() {

	db := initDB()

	session := initSession()

	wg := sync.WaitGroup{}

	app := Config{
		Session:  session,
		DB:       db,
		InfoLog:  log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime),
		ErrorLog: log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile),
		Wait:     &wg,
		Models:   data.New(db),
	}

	app.Mailer = app.createMail()
	go app.listenForMail()

	go app.listenForShutdown()

	app.serve()

}

func (app *Config) serve() {
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", webPort),
		Handler: app.routes(),
	}

	app.InfoLog.Printf("Starting server on %s", srv.Addr)
	err := srv.ListenAndServe()
	if err != nil {
		log.Panic(err)
	}
}

func initDB() *sql.DB {
	conn := connectToDB()
	if conn == nil {
		log.Panic("Failed to connect to the database")
	}
	return conn
}

func connectToDB() *sql.DB {
	counts := 0

	dsn := os.Getenv("DSN")

	for {
		connection, err := openDB(dsn)
		if err != nil {
			log.Println("Failed to connect to the database: ", err)
		} else {
			log.Println("Connected to the database")
			return connection
		}

		if counts > 10 {
			return nil
		}

		log.Println("Retrying to connect to the database")
		time.Sleep(1 * time.Second)
		counts++
		continue
	}
}

func openDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	return db, nil
}

func initSession() *scs.SessionManager {
	gob.Register(data.User{})
	session := scs.New()
	session.Store = redisstore.New(initRedis())
	session.Lifetime = 24 * time.Hour
	session.Cookie.Persist = true
	session.Cookie.SameSite = http.SameSiteLaxMode
	session.Cookie.Secure = true

	return session
}

func initRedis() *redis.Pool {
	redisPool := &redis.Pool{
		MaxIdle: 10,
		Dial: func() (redis.Conn, error) {
			return redis.Dial("tcp", os.Getenv("REDIS"))
		},
	}
	return redisPool
}

func (app *Config) listenForShutdown() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	app.shutdown()
	os.Exit(0)
}

func (app *Config) shutdown() {
	app.InfoLog.Println("Shutting down server")

	app.Wait.Wait()
	app.Mailer.DoneChan <- true
	app.InfoLog.Println("Closing channels and shutting dow application")
	close(app.Mailer.MailerChan)
	close(app.Mailer.ErrorChan)
	close(app.Mailer.DoneChan)
}

func (app *Config) createMail() Mail {
	errorChan := make(chan error)
	mailerdoneChan := make(chan bool)
	mailerChan := make(chan *Message, 100)

	m := Mail{
		Domain:      "localhost",
		Host:        "localhost",
		Port:        1025,
		Encryption:  "none",
		FromName:    "info",
		FromAddress: "info@example.com",
		Wait:        app.Wait,
		ErrorChan:   errorChan,
		DoneChan:    mailerdoneChan,
		MailerChan:  mailerChan,
	}

	return m
}
