package main

import (
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
	"strings"
)

const (
	QueryMailToSend = `SELECT snowflake, data FROM mail WHERE recipient = $1 AND is_sent = false ORDER BY snowflake`
	UpdateSentFlag  = `UPDATE mail SET is_sent = true WHERE snowflake = $1`
)

func receive(c *gin.Context) {
	mlid := c.PostForm("mlid")
	password := c.PostForm("passwd")

	err := validatePassword(mlid, password)
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
		return
	}

	mailSize := 0
	mailToSend := ""
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
		if len(mailToSend)+len(current) > maxSize {
			break
		}

		mailToSend += current
		numberOfMail++

		mailSize += len(data)

		_, err = pool.Exec(ctx, UpdateSentFlag, snowflake)
		if err != nil {
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
		mailToSend,
		"\r\n--", boundary, "--\r\n"))
}
