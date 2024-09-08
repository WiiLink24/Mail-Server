package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/logrusorgru/aurora/v4"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	QueryMailToSend = `SELECT snowflake, data FROM mail WHERE recipient = $1 AND is_sent = false ORDER BY snowflake LIMIT 10`
	UpdateSentFlag  = `UPDATE mail SET is_sent = true WHERE snowflake = $1`
)

func receive(c *gin.Context) {
	mlid := c.PostForm("mlid")
	password := c.PostForm("passwd")

	// Queries can take extremely long and eat up memory. Prevent this by enforcing a timeout.
	ctx, cancel := context.WithTimeout(c.Copy(), 10*time.Second)
	defer cancel()

	err := validatePassword(ctx, mlid, password)
	if errors.Is(err, ErrInvalidCredentials) {
		cgi := GenCGIError(250, err.Error())
		ReportError(err)
		c.String(http.StatusOK, ConvertToCGI(cgi))
		return
	} else if err != nil {
		cgi := GenCGIError(551, "An error has occurred while querying the database.")
		ReportError(err)
		c.String(http.StatusOK, ConvertToCGI(cgi))
		return
	}

	maxSize, err := strconv.Atoi(c.PostForm("maxsize"))
	if err != nil {
		cgi := GenCGIError(330, "maxsize needs to be an int.")
		c.String(http.StatusOK, ConvertToCGI(cgi))
		return
	}

	mail, err := pool.Query(ctx, QueryMailToSend, mlid[1:])
	if err != nil {
		cgi := GenCGIError(551, "An error has occurred while querying the database.")
		ReportError(err)
		c.String(http.StatusOK, ConvertToCGI(cgi))

		// Determine if this was a timeout error and log if so.
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			log.Printf("%s %s.", aurora.BgBrightYellow("Database query timed out for Wii"), mlid)
		}
		return
	}

	mailSize := 0
	mailToSend := new(strings.Builder)
	numberOfMail := 0

	boundary := generateBoundary()
	c.Header("Content-Type", fmt.Sprintf("multipart/mixed; boundary=%s", boundary))

	defer mail.Close()
	for mail.Next() {
		var snowflake int64
		var data string
		err = mail.Scan(&snowflake, &data)
		if err != nil {
			// Abandon this mail and report to Sentry
			ReportError(err)
			continue
		}

		// Upon testing with Doujinsoft, I realized that the Wii expects Windows (CRLF) newlines,
		// and will reject UNIX (LF) newlines.
		data = strings.Replace(data, "\n", "\r\n", -1)
		data = strings.Replace(data, "\r\r\n", "\r\n", -1)
		current := "\r\n--" + boundary + "\r\nContent-Type: text/plain\r\n\r\n" + data
		if mailToSend.Len()+len(current) > maxSize {
			break
		}

		mailToSend.WriteString(current)
		numberOfMail++

		mailSize += len(current)

		_, err = pool.Exec(ctx, UpdateSentFlag, snowflake)
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				log.Printf("%s %s.", aurora.BgBrightYellow("Database query timed out for Wii"), mlid)
			}

			ReportError(err)
		}
	}

	cgi := CGIResponse{
		code:    100,
		message: "Success.",
		other: []KV{
			{
				key:   "mailnum",
				value: strconv.Itoa(numberOfMail),
			},
			{
				key:   "mailsize",
				value: strconv.Itoa(mailSize),
			},
			{
				key:   "allnum",
				value: strconv.Itoa(numberOfMail),
			},
		},
	}

	if config.UseDatadog {
		err = dataDog.Incr("mail.received_mail", nil, float64(numberOfMail))
		if err != nil {
			ReportError(err)
		}
	}

	c.String(http.StatusOK, fmt.Sprint("--", boundary, "\r\n",
		"Content-Type: text/plain\r\n\r\n",
		"This part is ignored.\r\n\r\n\r\n\n",
		ConvertToCGI(cgi),
		mailToSend.String(),
		"\r\n--", boundary, "--\r\n"))
}
