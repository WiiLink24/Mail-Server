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
	"sync"
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
	c.Header("Content-Type", "text/plain;charset=utf-8")

	mlid, password := parseSendAuth(c.PostForm("mlid"))
	err := validatePassword(c.Copy(), mlid, password)
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

	wg := &sync.WaitGroup{}
	mu := &sync.Mutex{}

	for index, content := range mails {
		wg.Add(1)
		go func(index string, content string) {
			defer wg.Done()
			var wiiRecipients []string
			var emailRecipients []string
			var hasError bool

			// Read line by line
			msgIndex := 0
			scanner := bufio.NewScanner(strings.NewReader(content))
			for scanner.Scan() {
				line := scanner.Text()

				if line == "DATA" {
					// Find the index of the actual data and skip to the end
					scanner.Scan()
					line = scanner.Text()
					msgIndex = strings.Index(content, line)
					continue
				}

				if senderMatch := fromRegex.FindStringSubmatch(line); senderMatch != nil {
					if senderMatch[1] != mlid {
						mu.Lock()
						cgi.AddMailResponse(index, 350, "Attempted to impersonate another user.")
						mu.Unlock()
						hasError = true
						break
					}
					continue
				}

				if recipientMatch := recipientRegex.FindStringSubmatch(line); recipientMatch != nil {
					if recipientMatch[2] == "rc24.xyz" {
						wiiRecipients = append(wiiRecipients, recipientMatch[1])
					} else {
						emailRecipients = append(emailRecipients, fmt.Sprintf("%s@%s", recipientMatch[1], recipientMatch[2]))
					}
				}

				if senderMatch := fromRegexPayload.FindStringSubmatch(line); senderMatch != nil {
					if senderMatch[1] != mlid {
						mu.Lock()
						cgi.AddMailResponse(index, 350, "Attempted to impersonate another user.")
						mu.Unlock()
						hasError = true
					}
					break
				}
			}

			if hasError {
				return
			}

			parsedMail := content[msgIndex:]

			parsedMail = strings.Replace(parsedMail,
				"wii.com",
				"rc24.xyz",
				-1)

			var didError bool
			for _, recipient := range wiiRecipients {
				var exists bool
				err := pool.QueryRow(c.Copy(), RecipientExists, recipient[1:]).Scan(&exists)
				if err != nil && !errors.Is(err, sql.ErrNoRows) {
					mu.Lock()
					cgi.AddMailResponse(index, 551, "Issue verifying recipient.")
					mu.Unlock()
					ReportError(err)
					didError = true
					break
				} else if !exists {
					didError = true
					break
				}

				_, err = pool.Exec(c.Copy(), InsertMail, flakeNode.Generate(), parsedMail, mlid[1:], recipient[1:])
				if err != nil {
					mu.Lock()
					cgi.AddMailResponse(index, 450, "Database error.")
					mu.Unlock()
					ReportError(err)
					didError = true
					break
				}
			}

			if !didError {
				for _, recipient := range emailRecipients {
					auth := smtp.PlainAuth("", "apikey", config.SendGridKey, "smtp.sendgrid.net")
					err := smtp.SendMail(
						"smtp.sendgrid.net:587",
						auth,
						fmt.Sprintf("%s@rc24.xyz", mlid),
						[]string{recipient},
						[]byte(parsedMail),
					)
					if err != nil {
						mu.Lock()
						cgi.AddMailResponse(index, 551, "Sendgrid error.")
						mu.Unlock()
						didError = true
					}
				}
			}

			if !didError {
				mu.Lock()
				cgi.AddMailResponse(index, 100, "Success.")
				mu.Unlock()
				if config.UseDatadog {
					err = dataDog.Incr("mail.sent_mail", nil, 1)
					if err != nil {
						ReportError(err)
					}
				}
			}
		}(index, content)
	}

	wg.Wait()

	c.String(http.StatusOK, ConvertToCGI(*cgi))
}
