package main

import (
	"fmt"
	"github.com/WiiLink24/nwc24"
	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
	"github.com/logrusorgru/aurora/v4"
	"log"
	"math/rand"
	"strconv"
	"time"
)

const (
	letterBytes   = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
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

// ReportErrorGin helps make errors nicer. First it logs the error to Sentry,
// then prints the error to stdout
func ReportErrorGin(c *gin.Context, err error) {
	if hub := sentrygin.GetHubFromContext(c); hub != nil {
		hub.WithScope(func(scope *sentry.Scope) {
			hub.CaptureException(err)
		})
	}

	log.Printf("An error has occurred: %s", aurora.Red(err.Error()))
}

// ReportErrorGlobal is for errors that do not occur within a *gin.Context.
func ReportErrorGlobal(err error) {
	sentry.CaptureException(err)
	log.Printf("An error has occurred: %s", aurora.Red(err.Error()))
}

// generateBoundary returns a boundary string for use in the receive.cgi request.
func generateBoundary() string {
	source := rand.NewSource(time.Now().Unix())
	val := rand.New(source)
	return fmt.Sprintf("%s/%d", time.Now().Format("200601021504"), val.Intn(8999999)+1000000)
}

// validateFriendCode makes sure that the friend code is valid.
// This includes checking its crc and making sure it isn't the default Dolphin hollywood ID.
func validateFriendCode(strId string) bool {
	if len(strId) != 16 {
		// All Wii Numbers are 16 characters long.
		return false
	}

	id, err := strconv.ParseInt(strId, 10, 64)
	if err != nil {
		// Not an integer value, therefore not an ID
		return false
	}

	wiiNumber := nwc24.LoadWiiNumber(uint64(id))
	if !wiiNumber.CheckWiiNumber() {
		// Invalid Wii Number (crc is invalid)
		return false
	}

	return !(wiiNumber.GetHollywoodID() == 0x0403AC68)
}

// From: https://github.com/airylinus/goutils/blob/master/string.go
var src = rand.NewSource(time.Now().UnixNano())

// RandStringBytesMaskImprSrc - generate random string using masking with source
func RandStringBytesMaskImprSrc(n int) string {
	b := make([]byte, n)
	l := len(letterBytes)
	// A src.Int63() generates 63 random bits, enough for letterIdxMax characters!
	for i, cache, remain := n-1, src.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = src.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < l {
			b[i] = letterBytes[idx]
			i--
		}
		cache >>= letterIdxBits
		remain--
	}

	return string(b)
}
