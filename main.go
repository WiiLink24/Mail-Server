package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/bwmarrin/snowflake"
	"github.com/getsentry/sentry-go"
	"github.com/jackc/pgx/v4/pgxpool"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	"gopkg.in/DataDog/dd-trace-go.v1/profiler"
	"log"
	"net/http"
	"os"
	"time"
)

var (
	pool      *pgxpool.Pool
	config    *Config
	ctx       = context.Background()
	flakeNode *snowflake.Node
	dataDog   *statsd.Client
)

// checkError checks is an error is nil or not. Only to be used with functions that will cause
// the program not to continue.
func checkError(err error) {
	if err != nil {
		log.Fatalf("Wii Mail server has encountered an error! Reason: %v\n", err)
	}
}

func main() {
	rawConfig, err := os.ReadFile("./config.xml")
	checkError(err)

	config = &Config{}
	err = xml.Unmarshal(rawConfig, config)
	checkError(err)

	// Before we do anything, init Sentry to capture all errors.
	err = sentry.Init(sentry.ClientOptions{
		Dsn:              config.SentryDSN,
		Debug:            true,
		TracesSampleRate: 1.0,
	})
	checkError(err)
	defer sentry.Flush(2 * time.Second)

	if config.UseDatadog {
		// Initialize DataDog
		tracer.Start(
			tracer.WithService("mail"),
			tracer.WithEnv("prod"),
			tracer.WithAgentAddr("127.0.0.1:8126"),
		)
		defer tracer.Stop()

		err = profiler.Start(
			profiler.WithService("mail"),
			profiler.WithEnv("prod"),
		)
		checkError(err)
		defer profiler.Stop()

		dataDog, err = statsd.New("127.0.0.1:8125")
	}

	// Initialize snowflake
	flakeNode, err = snowflake.NewNode(1)
	checkError(err)

	// Initialize database
	dbString := fmt.Sprintf("postgres://%s:%s@%s/%s", config.SQLUser, config.SQLPass, config.SQLAddress, config.SQLDB)
	dbConf, err := pgxpool.ParseConfig(dbString)
	checkError(err)
	pool, err = pgxpool.ConnectConfig(ctx, dbConf)
	checkError(err)

	// Ensure this Postgresql connection is valid.
	defer pool.Close()

	fmt.Printf("Starting HTTP connection (%s)...\nNot using the usual port for HTTP?\nBe sure to use a proxy, otherwise the Wii can't connect!\n", config.Address)
	r := NewRoute()
	cgi := r.HandleGroup("cgi-bin")
	{
		cgi.Handle("check.cgi", check)
		cgi.Handle("send.cgi", send)
		cgi.Handle("receive.cgi", receive)
		cgi.Handle("delete.cgi", _delete)
		cgi.Handle("account.cgi", account)
	}

	mailGun := r.HandleGroup("mail")
	{
		mailGun.Handle("inbound", inbound)
	}

	log.Fatal(http.ListenAndServe(config.Address, r.Handle()))
}
