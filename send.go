package main

import (
	"bufio"
	"database/sql"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/smtp"
	"regexp"
	"strings"
)

var (
	sendAuthRegex    = regexp.MustCompile(`^mlid=(w\d{16})\r?\npasswd=(.{16,32})$`)
	mailFormKeyRegex = regexp.MustCompile(`m\d+`)
	recipientRegex   = regexp.MustCompile(`^RCPT TO:\s(.*)@(.*)$`)
	fromRegex        = regexp.MustCompile(`^MAIL FROM:\s(.*)@rc24.xyz$`)
	fromRegexPayload = regexp.MustCompile(`^From:\s(.*)@rc24.xyz$`)
)

const (
	RecipientExists = `SELECT EXISTS(SELECT 1 FROM accounts WHERE mlid = $1)`
	InsertMail      = `INSERT INTO mail (snowflake, data, sender, recipient, is_sent) VALUES ($1, $2, $3, $4, false)`
)

func send(c *gin.Context) {
	ctx := c.Copy()
	c.Header("Content-Type", "text/plain;charset=utf-8")

	mlid, password := parseSendAuth(c.PostForm("mlid"))
	err := validatePassword(ctx, mlid, password)
	if errors.Is(err, ErrInvalidCredentials) {
		cgi := GenCGIError(250, err.Error())
		c.String(http.StatusOK, ConvertToCGI(cgi))
		return
	} else if err != nil {
		cgi := GenCGIError(551, "An error has occurred while querying the database.")
		ReportError(err)
		c.String(http.StatusOK, ConvertToCGI(cgi))
		return
	}

	mails := make(map[string]string)

	form, err := c.MultipartForm()
	if err != nil {
		cgi := GenCGIError(250, err.Error())
		c.String(http.StatusOK, ConvertToCGI(cgi))
		return
	}

	for key, value := range form.Value {
		if mailFormKeyRegex.MatchString(key) {
			mails[key] = value[0]
		}
	}

	if len(mails) > 16 {
		cgi := GenCGIError(351, "Too many messages were sent.")
		c.String(http.StatusOK, ConvertToCGI(cgi))
		return
	}

	cgi := new(CGIResponse)
	cgi.code = 100
	cgi.message = "Success."

	for index, content := range mails {
		var wiiRecipients []string
		var emailRecipients []string
		var hasError bool

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
				continue
			}

			senderMatch := fromRegex.FindStringSubmatch(line)
			if senderMatch != nil {
				if senderMatch[1] != mlid {
					cgi.AddMailResponse(index, 350, "Attempted to impersonate another user.")
					hasError = true
					break
				}
				continue
			}

			recipientMatch := recipientRegex.FindStringSubmatch(line)
			if recipientMatch != nil {
				if recipientMatch[2] == "wii.com" || recipientMatch[2] == "mail.wiilink24.com" {
					// Theoretically this should not be possible.
					// A message formulated by a Wii used the address found in nwc24msg.cfg.
					// If we got far, it would be @rc24.xyz.
					// Regardless, if this does happen we don't want it clogging up our database or wasting
					// precious API calls.

					// Going back to my second comment, there was a moment where an attacker had the recipient
					// as WiiLink, causing it to spam both our clients. As such we should block any WiiLink recipients.
				} else if recipientMatch[2] == "rc24.xyz" {
					wiiRecipients = append(wiiRecipients, recipientMatch[1])
				} else {
					// This is an email.
					emailRecipients = append(emailRecipients, fmt.Sprintf("%s@%s", recipientMatch[1], recipientMatch[2]))
				}
			}

			// Additionally we also check the actual mail to see who it is coming from. A malicious user could
			// send a correct header, but spoof the actual mail, impersonating another user
			senderMatch = fromRegexPayload.FindStringSubmatch(line)
			if senderMatch != nil {
				if senderMatch[1] != mlid {
					cgi.AddMailResponse(index, 350, "Attempted to impersonate another user.")
					hasError = true
				}
				break
			}
		}

		if hasError {
			continue
		}

		parsedMail := content[msgIndex:]

		// Replace all @wii.com references in the
		// friend request email with our own domain.
		// Format: w9004342343324713@wii.com <mailto:w9004342343324713@wii.com>
		parsedMail = strings.Replace(parsedMail,
			"wii.com",
			"rc24.xyz",
			-1)

		var didError bool
		for _, recipient := range wiiRecipients {
			var exists bool
			err := pool.QueryRow(ctx, RecipientExists, recipient[1:]).Scan(&exists)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				cgi.AddMailResponse(index, 551, "Issue verifying recipient.")
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
				cgi.AddMailResponse(index, 450, "Database error.")
				ReportError(err)
				didError = true
				break
			}
		}

		for _, recipient := range emailRecipients {
			// PC Mail
			// We currently utilize SendGrid.
			auth := smtp.PlainAuth("", "apikey", config.SendGridKey, "smtp.sendgrid.net")
			err = smtp.SendMail(
				"smtp.sendgrid.net:587",
				auth,
				fmt.Sprintf("%s@rc24.xyz", mlid),
				[]string{recipient},
				[]byte(parsedMail),
			)
			if err != nil {
				cgi.AddMailResponse(index, 551, "Sendgrid error.")
				// ReportError(err)
				didError = true
				continue
			}
		}

		if !didError {
			// If everything was successful we write that to the response.
			cgi.AddMailResponse(index, 100, "Success.")

			if config.UseDatadog {
				err = dataDog.Incr("mail.sent_mail", nil, 1)
				if err != nil {
					ReportError(err)
				}
			}
		}
	}

	c.String(http.StatusOK, ConvertToCGI(*cgi))
}
