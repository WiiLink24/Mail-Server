package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	QueryMailToSend = `SELECT snowflake, data FROM mail WHERE recipient = $1 AND is_sent = false ORDER BY snowflake`
	UpdateSentFlag  = `UPDATE mail SET is_sent = true WHERE snowflake = $1`
)

func receive(r *Response) string {
	err := r.request.ParseForm()
	if err != nil {
		r.cgi = GenCGIError(350, "Failed to parse POST form.")
		return ConvertToCGI(r.cgi)
	}

	mlid := r.request.Form.Get("mlid")
	password := r.request.Form.Get("passwd")

	err = validatePassword(mlid, password)
	if errors.Is(err, ErrInvalidCredentials) {
		r.cgi = GenCGIError(250, err.Error())
		ReportError(err)
		return ConvertToCGI(r.cgi)
	} else if err != nil {
		r.cgi = GenCGIError(551, "An error has occurred while querying the database.")
		ReportError(err)
		return ConvertToCGI(r.cgi)
	}

	maxSize, err := strconv.Atoi(r.request.Form.Get("maxsize"))
	if err != nil {
		r.cgi = GenCGIError(330, "maxsize needs to be an int.")
		return ConvertToCGI(r.cgi)
	}

	mail, err := pool.Query(ctx, QueryMailToSend, mlid[1:])
	if err != nil {
		r.cgi = GenCGIError(551, "An error has occurred while querying the database.")
		ReportError(err)
		return ConvertToCGI(r.cgi)
	}

	mailSize := 0
	mailToSend := ""
	numberOfMail := 0

	boundary := generateBoundary()
	(*r.writer).Header().Add("Content-Type", fmt.Sprintf("multipart/mixed; boundary=%s", boundary))

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

	r.cgi = CGIResponse{
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

	err = dataDog.Incr("mail.received_mail", nil, float64(numberOfMail))
	if err != nil {
		ReportError(err)
	}

	return fmt.Sprint("--", boundary, "\r\n",
		"Content-Type: text/plain\r\n\r\n",
		"This part is ignored.\r\n\r\n\r\n\n",
		ConvertToCGI(r.cgi),
		mailToSend,
		"\r\n--", boundary, "--\r\n")
}
