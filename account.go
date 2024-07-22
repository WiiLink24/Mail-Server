package main

import (
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"net/http"
)

const CreateAccount = `INSERT INTO accounts (mlid, password, mlchkid) VALUES ($1, $2, $3)`

func account(c *gin.Context) {
	mlid := c.PostForm("mlid")
	if mlid == "" {
		cgi := GenCGIError(610, "mlid not found")
		c.String(http.StatusOK, ConvertToCGI(cgi))
		return
	}

	if !validateFriendCode(mlid[1:]) {
		cgi := GenCGIError(610, "Invalid Wii Friend Code")
		c.String(http.StatusOK, ConvertToCGI(cgi))
		return
	}

	c.Header("Content-Type", "text/plain;charset=utf-8")

	// Password can be any length up to 32 characters. 16 seems like a good middle ground.
	password := RandStringBytesMaskImprSrc(16)
	passwordByte := sha512.Sum512([]byte(password))
	passwordHash := hex.EncodeToString(passwordByte[:])

	// Mlchkid must be a string of 32 characters
	mlchkid := RandStringBytesMaskImprSrc(32)
	mlchkidByte := sha512.Sum512([]byte(mlchkid))
	mlchkidHash := hex.EncodeToString(mlchkidByte[:])

	_, err := pool.Exec(c.Copy(), CreateAccount, mlid[1:], passwordHash, mlchkidHash)
	if err != nil {
		var v *pgconn.PgError
		if errors.As(err, &v) {
			if pgerrcode.IsIntegrityConstraintViolation(v.Code) {
				cgi := GenCGIError(211, "Duplicate registration.")
				c.String(http.StatusOK, ConvertToCGI(cgi))
				return
			}
		}

		cgi := GenCGIError(410, "An error has occurred while querying the database.")
		ReportError(err)
		c.String(http.StatusOK, ConvertToCGI(cgi))
		return
	}

	cgi := CGIResponse{
		code:    100,
		message: "Success.",
		other: []KV{
			{
				key:   "mlid",
				value: mlid,
			},
			{
				key:   "passwd",
				value: password,
			},
			{
				key:   "mlchkid",
				value: mlchkid,
			},
		},
	}

	c.String(http.StatusOK, ConvertToCGI(cgi))
}
