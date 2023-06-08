package main

import (
	"crypto/sha512"
	"encoding/hex"
	"strconv"
)

const CreateAccount = `INSERT INTO accounts (mlid, password, mlchkid) VALUES ($1, $2, $3)`

func account(r *Response) string {
	mlid := r.request.Form.Get("mlid")
	if !validateFriendCode(mlid[1:]) {
		r.cgi = GenCGIError(610, "Invalid Wii Friend Code")
		return ConvertToCGI(r.cgi)
	} else if mlid == "" {
		r.cgi = GenCGIError(310, "Unable to parse parameters.")
		return ConvertToCGI(r.cgi)
	}

	(*r.writer).Header().Add("Content-Type", "text/plain;charset=utf-8")

	// Password can be any length up to 32 characters. 16 seems like a good middle ground.
	password := RandStringBytesMaskImprSrc(16)
	passwordByte := sha512.Sum512(append(salt, []byte(password)...))
	passwordHash := hex.EncodeToString(passwordByte[:])

	// Mlchkid must be a string of 32 characters
	mlchkid := RandStringBytesMaskImprSrc(32)
	mlchkidByte := sha512.Sum512(append(salt, []byte(mlchkid)...))
	mlchkidHash := hex.EncodeToString(mlchkidByte[:])

	id, _ := strconv.ParseInt(mlid[1:], 10, 64)

	result, err := pool.Exec(ctx, CreateAccount, id, passwordHash, mlchkidHash)
	if err != nil {
		r.cgi = GenCGIError(410, "An error has occurred while querying the database.")
		ReportError(err)
		return ConvertToCGI(r.cgi)
	}

	if result.RowsAffected() == 0 {
		r.cgi = GenCGIError(211, "Duplicate registration.")
		return ConvertToCGI(r.cgi)
	}
	r.cgi = CGIResponse{
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

	return ConvertToCGI(r.cgi)
}
