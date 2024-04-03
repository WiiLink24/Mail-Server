package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v4"
	"strconv"
)

const (
	DoesUserExist    = `SELECT mlid, last_flag FROM accounts WHERE mlchkid = $1`
	DoesUserHaveMail = `SELECT EXISTS(SELECT 1 FROM mail WHERE recipient = $1 AND is_sent = false)`
	InsertMailFlag   = `UPDATE accounts SET last_flag = $1 WHERE mlid = $2`
)

// MailHMACKey is the key used to sign the HMAC.
var MailHMACKey = []byte{0xce, 0x4c, 0xf2, 0x9a, 0x3d, 0x6b, 0xe1, 0xc2, 0x61, 0x91, 0x72, 0xb5, 0xcb, 0x29, 0x8c, 0x89, 0x72, 0xd4, 0x50, 0xad}

func check(r *Response) string {
	(*r.writer).Header().Add("X-Wii-Mail-Download-Span", "10")
	(*r.writer).Header().Add("X-Wii-Mail-Check-Span", "10")
	(*r.writer).Header().Add("X-Wii-Download-Span", "10")
	(*r.writer).Header().Add("Content-Type", "text/plain;charset=utf-8")

	mlchkid := r.request.Form.Get("mlchkid")
	if mlchkid == "" {
		r.cgi = GenCGIError(320, "Unable to find mlchkid.")
		return ConvertToCGI(r.cgi)
	}

	challenge := r.request.Form.Get("chlng")
	if challenge == "" {
		r.cgi = GenCGIError(320, "Unable to find chlng.")
		return ConvertToCGI(r.cgi)
	}

	var mlid uint64
	var lastFlag string
	password := hashPassword(mlchkid)
	row := pool.QueryRow(ctx, DoesUserExist, password)
	err := row.Scan(&mlid, &lastFlag)
	if errors.Is(err, pgx.ErrNoRows) {
		r.cgi = GenCGIError(321, "User does not exist.")
		return ConvertToCGI(r.cgi)
	} else if err != nil {
		r.cgi = GenCGIError(320, "Error has occurred in check query.")
		ReportError(err)
		return ConvertToCGI(r.cgi)
	}

	// The flag we send to the Wii is compared against the flag in wc24send.ctl. If it matches, no new mail is available.
	// If it doesn't, there is mail.
	var hasMail bool
	err = pool.QueryRow(ctx, DoesUserHaveMail, strconv.Itoa(int(mlid))).Scan(&hasMail)
	if err != nil {
		r.cgi = GenCGIError(320, "Error has occurred checking for mail.")
		ReportError(err)
		return ConvertToCGI(r.cgi)
	}

	var mailFlag string
	if hasMail {
		mailFlag = RandStringBytesMaskImprSrc(22)

		// Now insert the new flag.
		_, err = pool.Exec(ctx, InsertMailFlag, mailFlag, mlid)
		if err != nil {
			r.cgi = GenCGIError(320, "Error has occurred in saving mail flag.")
			ReportError(err)
			return ConvertToCGI(r.cgi)
		}
	} else {
		mailFlag = lastFlag
	}

	h := hmac.New(sha1.New, MailHMACKey)
	h.Write([]byte(challenge))
	h.Write([]byte("\n"))
	// We don't store the Wii Friend Code in the database with the w. The hash requires it.
	h.Write([]byte(fmt.Sprintf("w%016d", mlid)))
	h.Write([]byte("\n"))
	h.Write([]byte(mailFlag))
	h.Write([]byte("\n"))
	h.Write([]byte("10"))

	r.cgi = CGIResponse{
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

	err = dataDog.Incr("mail.checked", nil, 1)
	if err != nil {
		ReportError(err)
	}

	return ConvertToCGI(r.cgi)
}
