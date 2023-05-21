package main

import (
	"fmt"
	"github.com/getsentry/sentry-go"
	"github.com/logrusorgru/aurora/v4"
	"log"
	"math/rand"
	"strconv"
	"time"
)

func GenCGIError(code int, message string) CGIResponse {
	return CGIResponse{
		code:    code,
		message: message,
	}
}

func (c *CGIResponse) AddMailResponse(index string, code int, message string) {
	c.other = append(c.other, KV{
		key:   fmt.Sprintf("cd%s", index[1:]),
		value: strconv.Itoa(code),
	}, KV{
		key:   fmt.Sprintf("msg%s", index[1:]),
		value: message,
	})
}

// ReportError helps make errors nicer. First it logs the error to Sentry,
// then prints the error to stdout
func ReportError(err error) {
	sentry.CaptureException(err)
	log.Printf("An error has occurred: %s", aurora.Red(err.Error()))
}

// generateBoundary returns a boundary string for use in the receive.cgi request.
func generateBoundary() string {
	source := rand.NewSource(time.Now().Unix())
	val := rand.New(source)
	return fmt.Sprintf("%s/%d", time.Now().Format("200601021504"), val.Intn(8999999)+1000000)
}
