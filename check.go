package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v4"
	"net/http"
)

const (
	DoesUserExist    = `SELECT mlid FROM accounts WHERE mlchkid = $1`
	DoesUserHaveMail = `SELECT EXISTS(SELECT 1 FROM mail WHERE recipient = $1 AND is_sent = false)`
	NoMailFlag       = "000000000000000000000000000000000"
)

// MailHMACKey is the key used to sign the HMAC.
var MailHMACKey = []byte{0xce, 0x4c, 0xf2, 0x9a, 0x3d, 0x6b, 0xe1, 0xc2, 0x61, 0x91, 0x72, 0xb5, 0xcb, 0x29, 0x8c, 0x89, 0x72, 0xd4, 0x50, 0xad}

func check(c *gin.Context) {
	c.Header("X-Wii-Mail-Download-Span", "10")
	c.Header("X-Wii-Mail-Check-Span", "10")
	c.Header("X-Wii-Download-Span", "10")
	c.Header("Content-Type", "text/plain;charset=utf-8")

	mlchkid := c.PostForm("mlchkid")
	if mlchkid == "" {
		cgi := GenCGIError(320, "Unable to find mlchkid.")
		c.String(http.StatusOK, ConvertToCGI(cgi))
		return
	}

	challenge := c.PostForm("chlng")
	if challenge == "" {
		cgi := GenCGIError(320, "Unable to find chlng.")
		c.String(http.StatusOK, ConvertToCGI(cgi))
		return
	}

	var mlid string
	password := hashPassword(mlchkid)
	row := pool.QueryRow(ctx, DoesUserExist, password)
	err := row.Scan(&mlid)
	if errors.Is(err, pgx.ErrNoRows) {
		cgi := GenCGIError(321, "User does not exist.")
		c.String(http.StatusOK, ConvertToCGI(cgi))
		return
	} else if err != nil {
		cgi := GenCGIError(320, "Error has occurred in check query.")
		c.String(http.StatusOK, ConvertToCGI(cgi))
		return
	}

	// The flag we send to the Wii is compared against the flag in wc24send.ctl. If it matches, no new mail is available.
	// If it doesn't, there is mail.
	var hasMail bool
	err = pool.QueryRow(ctx, DoesUserHaveMail, mlid).Scan(&hasMail)
	if err != nil {
		cgi := GenCGIError(320, "Error has occurred checking for mail.")
		c.String(http.StatusOK, ConvertToCGI(cgi))
		return
	}

	mailFlag := NoMailFlag
	if hasMail {
		// Update mail flag
		mailFlag = RandStringBytesMaskImprSrc(33)
	}

	h := hmac.New(sha1.New, MailHMACKey)
	h.Write([]byte(challenge))
	h.Write([]byte("\n"))
	// We don't store the Wii Friend Code in the database with the w. The hash requires it.
	h.Write([]byte(fmt.Sprintf("w%s", mlid)))
	h.Write([]byte("\n"))
	h.Write([]byte(mailFlag))
	h.Write([]byte("\n"))
	h.Write([]byte("10"))

	cgi := CGIResponse{
		code:    100,
		message: "Success.",
		other: []KV{
			{
				key:   "res",
				value: hex.EncodeToString(h.Sum(nil)),
			},
			{
				key:   "mail.flag",
				value: mailFlag,
			},
			{
				key:   "interval",
				value: "10",
			},
		},
	}

	if config.UseDatadog {
		err = dataDog.Incr("mail.checked", nil, 1)
		if err != nil {
			ReportError(err)
		}
	}

	c.String(http.StatusOK, ConvertToCGI(cgi))
}
