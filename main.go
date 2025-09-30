package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"github.com/DataDog/datadog-go/v5/statsd"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bwmarrin/snowflake"
	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	"gopkg.in/DataDog/dd-trace-go.v1/profiler"
	"log"
	"os"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

var (
	s3Client  *s3.Client
	ctx       = context.Background()
	pool      *pgxpool.Pool
	config    *Config
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

	s3Config, err := awsConfig.LoadDefaultConfig(context.TODO(),
		awsConfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(config.AWSAccessID, config.AWSSecretKey, "")),
		awsConfig.WithRegion(config.AWSRegion),
	)
	checkError(err)

	s3Client = s3.NewFromConfig(s3Config)

	// Initialize database
	dbString := fmt.Sprintf("postgres://%s:%s@%s/%s", config.SQLUser, config.SQLPass, config.SQLAddress, config.SQLDB)
	dbConf, err := pgxpool.ParseConfig(dbString)
	checkError(err)
	pool, err = pgxpool.NewWithConfig(ctx, dbConf)
	checkError(err)

	// Ensure this Postgresql connection is valid.
	defer pool.Close()

	fmt.Printf("Starting HTTP connection (%s)...\nNot using the usual port for HTTP?\nBe sure to use a proxy, otherwise the Wii can't connect!\n", config.Address)
	g := gin.Default()

	if config.UseOTLP {
		tp, err := initTracer(config)
		if err != nil {
			log.Fatalf("Failed to initialize tracer: %v", err)
		}
		defer func() {
			if err = tp.Shutdown(ctx); err != nil {
				log.Printf("Error shutting down tracer provider: %v", err)
			}
		}()

		g.Use(otelgin.Middleware("wii-mail", otelgin.WithTracerProvider(tp)))
	}

	g.Use(sentrygin.New(sentrygin.Options{}))

	g.POST("/cgi-bin/check.cgi", check)
	g.POST("/cgi-bin/send.cgi", send)
	g.POST("/cgi-bin/receive.cgi", receive)
	g.POST("/cgi-bin/delete.cgi", _delete)
	g.POST("/cgi-bin/account.cgi", account)

	go processInbound()
	log.Fatalln(g.Run(config.Address))
}
