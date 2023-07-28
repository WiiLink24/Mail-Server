package main

import (
	"database/sql"
	"fmt"
	"net/smtp"
	"regexp"
	"strings"
)

var (
	sendAuthRegex    = regexp.MustCompile(`^mlid=(w\d{16})\r?\npasswd=(.{16,32})$`)
	mailFormKeyRegex = regexp.MustCompile(`m\d+`)
	topMailRegex     = regexp.MustCompile(`^MAIL FROM: (w\d{16})@wiilink24.com\r?\nRCPT TO: (.*)@(.*)\r?\nDATA\r?\nDate: .*\r?\nFrom: (w\d{16})@wiilink24.com\r?\nTo: (.*)@(.*)\r?\n`)
)

const (
	RecipientExists = `SELECT EXISTS(SELECT 1 FROM accounts WHERE mlid = $1)`
	InsertMail      = `INSERT INTO mail (snowflake, data, sender, recipient, is_sent) VALUES ($1, $2, $3, $4, false)`
)

func send(r *Response) string {
	(*r.writer).Header().Add("Content-Type", "text/plain;charset=utf-8")

	err := r.request.ParseMultipartForm(-1)
	if err != nil {
		r.cgi = GenCGIError(350, "Failed to parse mail.")
		ReportError(err)
		return ConvertToCGI(r.cgi)
	}

	mlid, password := parseSendAuth(r.request.Form.Get("mlid"))
	err = validatePassword(mlid, password)
	if err == ErrInvalidCredentials {
		r.cgi = GenCGIError(250, err.Error())
		return ConvertToCGI(r.cgi)
	} else if err != nil {
		r.cgi = GenCGIError(551, "An error has occurred while querying the database.")
		ReportError(err)
		return ConvertToCGI(r.cgi)
	}

	mails := make(map[string]string)

	for key, value := range r.request.MultipartForm.Value {
		if mailFormKeyRegex.MatchString(key) {
			mails[key] = value[0]
		}
	}

	if len(mails) > 16 {
		r.cgi = GenCGIError(351, "Too many messages were sent.")
		return ConvertToCGI(r.cgi)
	}

	r.cgi.code = 100
	r.cgi.message = "Success."

	for index, content := range mails {
		var recipient string

		// Validate that the mail has all the metadata we need
		isRecipientWii := false
		meta := topMailRegex.FindStringSubmatch(content)
		if meta != nil {
			// First Match - Sender
			// Second Match - Recipient without address
			// Third match - Recipient's address
			// Forth Match - Sender but in MIME format
			// Fifth Match - Recipient but in MIME format without address
			// Sixth Match - Recipient's address but in MIME format
			if meta[1] != meta[4] || meta[1] != mlid || meta[4] != mlid {
				r.cgi.AddMailResponse(index, 350, "Attempted to impersonate another user.")
				continue
			}

			if meta[2] != meta[5] || meta[3] != meta[6] {
				r.cgi.AddMailResponse(index, 350, "Recipients do not match.")
				continue
			}

			if meta[3] == "wii.com" || meta[3] == "wiilink24.com" {
				isRecipientWii = true
				recipient = meta[2]
			} else {
				recipient = meta[2] + "@" + meta[3]
			}
		}

		parsedMail := content[strings.Index(content, "DATA")+5:]

		// Replace all @wii.com references in the
		// friend request email with our own domain.
		// Format: w9004342343324713@wii.com <mailto:w9004342343324713@wii.com>
		parsedMail = strings.Replace(parsedMail,
			fmt.Sprintf("%s@wii.com <mailto:%s@wii.com>", mlid, mlid),
			fmt.Sprintf("%s@wiilink24.com <mailto:%s@wiilink24.com>", mlid, mlid),
			-1)

		if isRecipientWii {
			var exists bool
			err := pool.QueryRow(ctx, RecipientExists, recipient[1:]).Scan(&exists)
			if err != nil && err != sql.ErrNoRows {
				r.cgi.AddMailResponse(index, 551, "Issue verifying recipient.")
				ReportError(err)
				continue
			} else if !exists {
				// Account doesn't exist, ignore
				continue
			}

			// Finally insert!
			_, err = pool.Exec(ctx, InsertMail, flakeNode.Generate(), parsedMail, mlid[1:], recipient[1:])
			if err != nil {
				r.cgi.AddMailResponse(index, 450, "Database error.")
				ReportError(err)
				continue
			}
		} else {
			// PC Mail
			// We currently utilize SendGrid, TODO: Use MailGun we get 20k messages/month
			auth := smtp.PlainAuth("", "apikey", config.SendGridKey, "smtp.sendgrid.net")
			err = smtp.SendMail(
				"smtp.sendgrid.net:587",
				auth,
				fmt.Sprintf("%s@wiilink24.com", mlid),
				[]string{recipient},
				[]byte(parsedMail),
			)
			if err != nil {
				r.cgi.AddMailResponse(index, 551, "Sendgrid error.")
				ReportError(err)
				continue
			}
		}

		// If everything was successful we write that to the response.
		r.cgi.AddMailResponse(index, 100, "Success.")
	}

	return ConvertToCGI(r.cgi)
}
