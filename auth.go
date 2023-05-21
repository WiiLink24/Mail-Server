package main

import (
	"crypto/sha512"
	"encoding/hex"
	"errors"
)

var (
	ErrInvalidCredentials = errors.New("an authentication error occurred")
)

const ValidatePassword = `SELECT password FROM accounts WHERE mlid = $1 AND password = $2`

// hashPassword hashes the mlchkid for usage in the database.
func hashPassword(password string) string {
	hashByte := sha512.Sum512(append(salt, []byte(password)...))
	return hex.EncodeToString(hashByte[:])
}

// From https://github.com/RiiConnect24/Mail-Go/blob/master/auth.go#L28
// parseSendAuth obtains a mlid and passwd from the given format.
// If it is unable to do so, it returns empty strings for both.
// It additionally determines whether the given mlid is valid -
// if not, it returns empty strings for both values as well.
func parseSendAuth(format string) (string, string) {
	match := sendAuthRegex.FindStringSubmatch(format)
	if match != nil {
		// Format:
		// [0] = raw string
		// [1] = mlid match
		// [2] = passwd match
		return match[1], match[2]
	} else {
		return "", ""
	}
}

func validatePassword(mlid, password string) error {
	if mlid == "" || password == "" {
		return ErrInvalidCredentials
	}

	hash := hashPassword(password)
	row := pool.QueryRow(ctx, ValidatePassword, mlid[1:], hash)
	err := row.Scan(nil)
	if err != nil {
		return err
	}

	return nil
}
