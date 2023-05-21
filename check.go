package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"github.com/jackc/pgx/v4"
)

const (
	DoesUserExist    = `SELECT mlid FROM accounts WHERE mlchkid = $1`
	DoesUserHaveMail = `SELECT snowflake FROM mail WHERE recipient = $1 AND is_sent = false LIMIT 1`
)

// MailHMACKey is the key used to sign the HMAC.
var MailHMACKey = []byte{0xce, 0x4c, 0xf2, 0x9a, 0x3d, 0x6b, 0xe1, 0xc2, 0x61, 0x91, 0x72, 0xb5, 0xcb, 0x29, 0x8c, 0x89, 0x72, 0xd4, 0x50, 0xad}

func check(r *Response) string {
	(*r.writer).Header().Add("X-Wii-Mail-Download-Span", "10")
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
	password := hashPassword(mlchkid)
	row := pool.QueryRow(ctx, DoesUserExist, password)
	err := row.Scan(&mlid)
	if err == pgx.ErrNoRows {
		r.cgi = GenCGIError(321, "User does not exist.")
		return ConvertToCGI(r.cgi)
	} else if err != nil {
		r.cgi = GenCGIError(320, "Error has occurred in check query.")
		ReportError(err)
		return ConvertToCGI(r.cgi)
	}

	// The set mail flag can be literally anything other than the string literal 0000000000000000000000.
	// Although code exists to update the mail flag, it is never called.
	// KD compares this with the mail flag we send. If it matches, it will not try to receive mail. Otherwise, it does.
	mailFlag := "1000000000000000000000"
	row = pool.QueryRow(context.Background(), DoesUserHaveMail, mlid)
	err = row.Scan(nil)
	if err == pgx.ErrNoRows {
		mailFlag = "0000000000000000000000"
	} else if err != nil {
		r.cgi = GenCGIError(320, "Error has occurred checking for mail.")
		ReportError(err)
		return ConvertToCGI(r.cgi)
	}

	h := hmac.New(sha1.New, MailHMACKey)
	h.Write([]byte(challenge))
	h.Write([]byte("\n"))
	// We don't store the Wii Friend Code in the database with the w. The hash requires it.
	h.Write([]byte(fmt.Sprintf("w%d", mlid)))
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

	return ConvertToCGI(r.cgi)
}
