package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"net/smtp"
	"regexp"
	"strings"
)

var (
	sendAuthRegex    = regexp.MustCompile(`^mlid=(w\d{16})\r?\npasswd=(.{16,32})$`)
	mailFormKeyRegex = regexp.MustCompile(`m\d+`)
	recipientRegex   = regexp.MustCompile(`^RCPT TO:\s(.*)@(.*)$`)
	fromRegex        = regexp.MustCompile(`^MAIL FROM:\s(.*)@.*$`)
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
		var wiiRecipients []string
		var emailRecipients []string

		// Read line by line.
		// If you look in Git history, you can see that I used a method that was faster than this current one,
		// then I learnt Wii Speak is able to send to multiple recipients at once.
		msgIndex := 0
		scanner := bufio.NewScanner(strings.NewReader(content))
		for scanner.Scan() {
			line := scanner.Text()

			if line == "DATA" {
				// We have reached the end of metadata parsing.
				// Find the index of the actual data and skip to the end.
				scanner.Scan()
				line = scanner.Text()
				msgIndex = strings.Index(content, line)
				break
			}

			senderMatch := fromRegex.FindStringSubmatch(line)
			if senderMatch != nil {
				if senderMatch[1] != mlid {
					r.cgi.AddMailResponse(index, 350, "Attempted to impersonate another user.")
				}
				continue
			}

			recipientMatch := recipientRegex.FindStringSubmatch(line)
			if recipientMatch != nil {
				if recipientMatch[2] == "wii.com" {
					// Theoretically this should not be possible.
					// A message formulated by a Wii used the address found in nwc24msg.cfg.
					// If we got far, it would be @wiilink24.com.
					// Regardless, if this does happen we don't want it clogging up our database or wasting
					// precious API calls.
				} else if recipientMatch[2] == "mail.wiilink24.com" {
					wiiRecipients = append(wiiRecipients, recipientMatch[1])
				} else {
					// This is an email.
					emailRecipients = append(emailRecipients, fmt.Sprintf("%s@%s", recipientMatch[1], recipientMatch[2]))
				}
			}
		}

		parsedMail := content[msgIndex:]

		// Replace all @wii.com references in the
		// friend request email with our own domain.
		// Format: w9004342343324713@wii.com <mailto:w9004342343324713@wii.com>
		parsedMail = strings.Replace(parsedMail,
			fmt.Sprintf("%s@wii.com <mailto:%s@wii.com>", mlid, mlid),
			fmt.Sprintf("%s@mail.wiilink24.com <mailto:%s@mail.wiilink24.com>", mlid, mlid),
			-1)

		var didError bool
		for _, recipient := range wiiRecipients {
			var exists bool
			err := pool.QueryRow(ctx, RecipientExists, recipient[1:]).Scan(&exists)
			if err != nil && err != sql.ErrNoRows {
				r.cgi.AddMailResponse(index, 551, "Issue verifying recipient.")
				ReportError(err)
				didError = true
				break
			} else if !exists {
				// Account doesn't exist, ignore
				didError = true
				break
			}

			// Finally insert!
			_, err = pool.Exec(ctx, InsertMail, flakeNode.Generate(), parsedMail, mlid[1:], recipient[1:])
			if err != nil {
				r.cgi.AddMailResponse(index, 450, "Database error.")
				ReportError(err)
				didError = true
				break
			}
		}

		for _, recipient := range emailRecipients {
			// PC Mail
			// We currently utilize SendGrid, TODO: Use MailGun we get 20k messages/month
			auth := smtp.PlainAuth("", "postmaster@mail.wiilink24.com", config.MailGunKey, "smtp.mailgun.org")
			err = smtp.SendMail(
				"smtp.mailgun.org:587",
				auth,
				fmt.Sprintf("%s@mail.wiilink24.com", mlid),
				[]string{recipient},
				[]byte(parsedMail),
			)
			if err != nil {
				r.cgi.AddMailResponse(index, 551, "MailGun error.")
				ReportError(err)
				didError = true
				continue
			}
		}

		if !didError {
			// If everything was successful we write that to the response.
			r.cgi.AddMailResponse(index, 100, "Success.")
		}
	}

	return ConvertToCGI(r.cgi)
}
