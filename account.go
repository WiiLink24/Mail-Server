package main

import (
	"crypto/sha512"
	"encoding/hex"
)

const CreateAccount = `INSERT INTO accounts (mlid, password, mlchkid) VALUES ($1, $2, $3)`

func account(r *Response) string {
	err := r.request.ParseForm()
	if err != nil {
		r.cgi = GenCGIError(350, "Failed to parse POST form.")
		return ConvertToCGI(r.cgi)
	}

	mlid := r.request.Form.Get("mlid")
	if mlid == "" {
		r.cgi = GenCGIError(610, "mlid not found")
		return ConvertToCGI(r.cgi)
	}

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
	passwordByte := sha512.Sum512([]byte(password))
	passwordHash := hex.EncodeToString(passwordByte[:])

	// Mlchkid must be a string of 32 characters
	mlchkid := RandStringBytesMaskImprSrc(32)
	mlchkidByte := sha512.Sum512([]byte(mlchkid))
	mlchkidHash := hex.EncodeToString(mlchkidByte[:])

	result, err := pool.Exec(ctx, CreateAccount, mlid[1:], passwordHash, mlchkidHash)
	if result.RowsAffected() == 0 {
		r.cgi = GenCGIError(211, "Duplicate registration.")
		return ConvertToCGI(r.cgi)
	}

	if err != nil {
		r.cgi = GenCGIError(410, "An error has occurred while querying the database.")
		ReportError(err)
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
